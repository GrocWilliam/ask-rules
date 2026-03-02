// pipeline/pipeline.go — Orchestration du pipeline d'indexation
package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"

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

	emit("start", map[string]interface{}{"game": opts.GameName, "files": len(opts.FilePaths)})

	// Chercher ou créer l'entrée en base
	game, err := db.FindGameByName(ctx, opts.GameName)
	if err != nil || game == nil {
		return fmt.Errorf("jeu introuvable en base : %s", opts.GameName)
	}

	totalChunks := 0
	// Accumuler tous les textes pour l'extraction de gameplay
	var allChunkTexts []string

	for _, fp := range opts.FilePaths {
		chunkTexts, fileErr := processFile(ctx, game, fp, emit, &totalChunks)
		if fileErr != nil {
			emit("file_error", map[string]interface{}{"file": fp, "error": fileErr.Error()})
			// continuer avec les autres fichiers
		}
		allChunkTexts = append(allChunkTexts, chunkTexts...)
	}

	// ── Extraction du gameplay ─────────────────────────────────────────────
	emit("gameplay", map[string]interface{}{"status": "extracting", "chunks": len(allChunkTexts)})

	gameplayData := nlp.ExtractGameplay(allChunkTexts)
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

	emit("done", map[string]interface{}{
		"game":         opts.GameName,
		"total_chunks": totalChunks,
	})
	return nil
}

func processFile(ctx context.Context, game *models.Game, fp string, emit func(string, map[string]interface{}), total *int) ([]string, error) {
	absPath := storage.GetAbsolutePath(fp)

	emit("extracting", map[string]interface{}{"file": fp})
	text, pages, err := ExtractText(absPath)
	if err != nil {
		return nil, fmt.Errorf("extraction: %w", err)
	}
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("fichier vide: %s", fp)
	}

	emit("chunking", map[string]interface{}{"file": fp, "text_length": len(text)})
	chunks := ChunkText(text, pages)

	emit("embedding", map[string]interface{}{"file": fp, "chunks": len(chunks)})

	var chunkTexts []string
	for i, chunk := range chunks {
		select {
		case <-ctx.Done():
			return chunkTexts, ctx.Err()
		default:
		}

		// Embedding
		vec, embErr := embedder.Embed(ctx, chunk.Text)
		if embErr != nil {
			// Continuer sans embedding
			vec = nil
		}

		// NLP
		sectionType := nlp.DetectSectionType("", chunk.Text)
		mechanics := nlp.DetectMechanics(chunk.Text)
		keywords := nlp.ExtractKeywords(chunk.Text)
		_ = keywords // stocké dans metadata de la section

		section := models.Section{
			ID:          newUUID(),
			GameID:      game.ID,
			Title:       fmt.Sprintf("Extrait %d", i+1),
			Level:       1,
			SectionType: sectionType,
			Text:        chunk.Text,
			Entities:    []string{},
			Actions:     []string{},
			Summary:     chunk.Text[:min(160, len(chunk.Text))],
			Mechanics:   mechanics,
			Embedding:   vecToFloat64(vec),
			PageStart:   intPtr(chunk.PageStart),
			PageEnd:     intPtr(chunk.PageEnd),
			HierarchyPath: fp,
			ChunkIndex:  i,
			TotalChunks: len(chunks),
		}

		if err := db.InsertSection(ctx, section); err != nil {
			emit("section_error", map[string]interface{}{"index": i, "error": err.Error()})
		} else {
			*total++
			chunkTexts = append(chunkTexts, chunk.Text)
		}

		if i%10 == 0 {
			emit("progress", map[string]interface{}{"file": fp, "done": i + 1, "total": len(chunks)})
		}
	}

	return chunkTexts, nil
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
