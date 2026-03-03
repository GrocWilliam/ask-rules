// pipeline/pipeline.go — Orchestration du pipeline d'indexation
package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"

	"ask-rules-server/config"
	"ask-rules-server/db"
	"ask-rules-server/embedder"
	"ask-rules-server/models"
	"ask-rules-server/nlp"
	"ask-rules-server/storage"
)

// ImportOptions paramètres d'import.
type ImportOptions struct {
	GameName  string
	FilePaths []string // chemins relatifs dans UPLOADS_DIR
	OnEvent   func(event string, data map[string]interface{})
}

// Run exécute le pipeline complet pour un jeu.
// Retourne statistiques et erreur.
func Run(ctx context.Context, opts ImportOptions) error {
	emit := opts.OnEvent
	if emit == nil {
		emit = func(string, map[string]interface{}) {}
	}

	// Vérifier que l'embedder est prêt avant de démarrer
	if err := embedder.Init(); err != nil {
		return fmt.Errorf("embedder non prêt (chemin modèle : %s) : %w", config.C.ModelPath, err)
	}

	emit("start", map[string]interface{}{"game": opts.GameName, "files": len(opts.FilePaths)})

	// Chercher ou créer l'entrée en base
	game, err := db.FindGameByName(ctx, opts.GameName)
	if err != nil || game == nil {
		return fmt.Errorf("jeu introuvable en base : %s", opts.GameName)
	}

	totalChunks := 0
	// Limiter la taille du buffer pour l'extraction de gameplay (éviter surcharge RAM)
	// 150 chunks = suffisant pour 95% des jeux (livrets < 100 pages)
	// Pour jeux > 200 chunks, seuls les premiers sont analysés (setup/tour généralement au début)
	// Impact : 0-5% précision gameplay, 0% précision recherche/réponses (voir IMPACT_PRECISION.md)
	const maxChunksForGameplay = 150 // ~90KB de texte max en mémoire
	var gameplayChunks []string
	var metaChunks []string

	for _, fp := range opts.FilePaths {
		chunkCount := processFileStreaming(ctx, game, fp, emit, &totalChunks, &gameplayChunks, &metaChunks, maxChunksForGameplay)
		if chunkCount < 0 {
			emit("file_error", map[string]interface{}{"file": fp, "error": "Erreur traitement fichier"})
			continue
		}
	}

	// ── Extraction des métadonnées du jeu ─────────────────────────────────────
	if len(metaChunks) > 0 {
		gameMeta := nlp.ExtractGameMeta(metaChunks)
		if len(gameMeta) > 0 {
			if game.Metadata == nil {
				game.Metadata = map[string]interface{}{}
			}
			for k, v := range gameMeta {
				game.Metadata[k] = v
			}
			_ = db.UpsertGame(ctx, game)
		}
	}
	metaChunks = nil // Libérer mémoire

	// ── Extraction du gameplay ─────────────────────────────────────────────
	if len(gameplayChunks) > 0 {
		emit("gameplay", map[string]interface{}{"status": "extracting", "chunks": len(gameplayChunks)})
		gameplayData := nlp.ExtractGameplay(gameplayChunks)
		gameplayChunks = nil // Libérer mémoire immédiatement

		if !gameplayData.IsEmpty() {
			if gMap := gameplayDataToMap(gameplayData); gMap != nil {
				if saveErr := db.UpdateGameplay(ctx, game.ID, gMap); saveErr != nil {
					emit("gameplay_error", map[string]interface{}{"error": saveErr.Error()})
				} else {
					emit("gameplay", map[string]interface{}{
						"status":    "done",
						"mechanics": len(gameplayData.Mechanics),
						"phases":    len(gameplayData.Phases),
						"has_setup": gameplayData.Setup != nil,
						"has_turns": gameplayData.Turns != nil,
						"has_end":   gameplayData.EndGame != nil,
					})
				}
			}
		}
	}

	emit("done", map[string]interface{}{
		"game":         opts.GameName,
		"total_chunks": totalChunks,
	})
	return nil
}

// processFileStreaming traite un fichier en streaming sans accumuler tous les chunks.
// Ajoute sélectivement les chunks aux buffers gameplayChunks et metaChunks.
// Retourne le nombre de chunks traités, ou -1 en cas d'erreur.
func processFileStreaming(
	ctx context.Context,
	game *models.Game,
	fp string,
	emit func(string, map[string]interface{}),
	total *int,
	gameplayChunks *[]string,
	metaChunks *[]string,
	maxChunks int,
) int {
	absPath := storage.GetAbsolutePath(fp)

	emit("extracting", map[string]interface{}{"file": fp})
	text, pages, err := ExtractText(absPath)
	if err != nil {
		return -1
	}
	if strings.TrimSpace(text) == "" {
		return -1
	}

	emit("chunking", map[string]interface{}{"file": fp, "text_length": len(text)})
	chunks := ChunkText(text, pages)

	// Libérer le texte source immédiatement
	text = ""
	pages = nil

	emit("embedding", map[string]interface{}{"file": fp, "chunks": len(chunks)})

	processedCount := 0
	for i, chunk := range chunks {
		select {
		case <-ctx.Done():
			return processedCount
		default:
		}

		// Nettoyer les séquences UTF-8 invalides (pdftotext peut en produire)
		cleanText := sanitizeUTF8(chunk.Text)

		// Ignorer les chunks de mauvaise qualité (tables des matières, scores, artefacts PDF)
		if IsLowQualityText(cleanText) {
			continue
		}

		vec, embErr := embedder.Embed(ctx, cleanText)
		if embErr != nil {
			emit("section_error", map[string]interface{}{"index": i, "error": "embedding: " + embErr.Error()})
			vec = nil
		}

		// NLP
		sectionType := nlp.DetectSectionType("", cleanText)
		mechanics := nlp.DetectMechanics(cleanText)

		section := models.Section{
			ID:            newUUID(),
			GameID:        game.ID,
			Title:         generateTitle(sectionType, cleanText, i),
			SectionType:   sectionType,
			Text:          cleanText,
			Summary:       extractFirstSentence(cleanText, 220),
			Mechanics:     mechanics,
			Embedding:     vecToFloat64(vec),
			PageStart:     intPtr(chunk.PageStart),
			PageEnd:       intPtr(chunk.PageEnd),
			HierarchyPath: sectionType,
			ChunkIndex:    i,
			TotalChunks:   len(chunks),
		}

		if err := db.InsertSection(ctx, section); err != nil {
			emit("section_error", map[string]interface{}{"index": i, "error": err.Error()})
		} else {
			*total++
			processedCount++

			// Garder en mémoire seulement les chunks nécessaires pour metadata/gameplay
			if len(*metaChunks) < 5 {
				*metaChunks = append(*metaChunks, cleanText)
			}
			if len(*gameplayChunks) < maxChunks {
				*gameplayChunks = append(*gameplayChunks, cleanText)
			}
		}

		if i%10 == 0 {
			emit("progress", map[string]interface{}{"file": fp, "done": i + 1, "total": len(chunks)})
		}
	}

	return processedCount
}

// gameplayDataToMap convertit GameplayData en map[string]interface{} via JSON
func gameplayDataToMap(gd *nlp.GameplayData) map[string]interface{} {
	b, err := json.Marshal(gd)
	if err != nil {
		return nil
	}
	var m map[string]interface{}
	_ = json.Unmarshal(b, &m)
	return m
}

func vecToFloat64(v []float32) []float64 {
	if v == nil {
		return nil
	}
	out := make([]float64, len(v))
	for i, f := range v {
		out[i] = float64(f)
	}
	return out
}

func intPtr(v int) *int { return &v }

func newUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// sanitizeUTF8 supprime les séquences d'octets invalides en UTF-8.
func sanitizeUTF8(s string) string {
	return strings.ToValidUTF8(s, "")
}

// generateTitle crée un titre lisible pour un chunk à partir de :
//  1. Le type de section détecté (label FR)
//  2. Les premiers mots significatifs du texte (première phrase tronquée)
func generateTitle(sectionType, text string, index int) string {
	sectionLabels := map[string]string{
		"setup":     "Mise en place",
		"turn":      "Tour de jeu",
		"scoring":   "Score & Victoire",
		"end":       "Fin de partie",
		"component": "Composants",
		"special":   "Règle spéciale",
		"example":   "Exemple",
	}

	// Extraire la première ligne non vide significative (titre de section dans le PDF)
	firstLine := ""
	for _, line := range strings.SplitN(text, "\n", 10) {
		line = strings.TrimSpace(line)
		runes := []rune(line)
		// Ligne courte (≤ 80 car.) non purement numérique → probablement un titre de section
		if len(runes) >= 4 && len(runes) <= 80 {
			allDigitsOrPunct := true
			letterCount := 0
			for _, r := range runes {
				if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r > 127 {
					allDigitsOrPunct = false
					letterCount++
				}
			}
			if !allDigitsOrPunct && letterCount >= 3 {
				firstLine = line
				break
			}
		}
	}

	typeLabel, hasType := sectionLabels[sectionType]

	if firstLine != "" {
		// Tronquer si trop long
		runes := []rune(firstLine)
		if len(runes) > 60 {
			firstLine = string(runes[:60]) + "…"
		}
		if hasType && sectionType != "general" {
			return typeLabel + " — " + firstLine
		}
		return firstLine
	}

	// Fallback : label du type seul
	if hasType && sectionType != "general" {
		return fmt.Sprintf("%s %d", typeLabel, index+1)
	}
	return fmt.Sprintf("Règles %d", index+1)
}

// truncateRunes tronque s à maxRunes caractères Unicode (pas bytes).
func truncateRunes(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes])
}

// extractFirstSentence retourne la première phrase complète du texte (terminée par
// '.', '!', '?' ou un saut de ligne double), tronquée à maxRunes si nécessaire.
// Le résultat est donc factuellement distinct du contenu complet.
func extractFirstSentence(s string, maxRunes int) string {
	s = strings.TrimSpace(s)
	// Cherche la fin de la première phrase
	for i, r := range s {
		if r == '.' || r == '!' || r == '?' {
			sentence := strings.TrimSpace(s[:i+1])
			return truncateRunes(sentence, maxRunes)
		}
		// Paragraphe double-saut de ligne
		if i > 0 && strings.HasPrefix(s[i:], "\n\n") {
			sentence := strings.TrimSpace(s[:i])
			if sentence != "" {
				return truncateRunes(sentence, maxRunes)
			}
		}
	}
	// Pas de ponctuation trouvée → troncature propre au dernier espace
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	cut := string(runes[:maxRunes])
	// Remonte au dernier espace pour ne pas couper un mot
	if idx := strings.LastIndexByte(cut, ' '); idx > maxRunes/2 {
		cut = cut[:idx]
	}
	return strings.TrimSpace(cut) + "…"
}
