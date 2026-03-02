// context/builder.go — Construction du contexte LLM
package context

import (
	"fmt"
	"strings"

	"ask-rules-server/models"
)

const (
	maxContextChars  = 3200
	maxSections      = 4
	maxGameplayChars = 600 // budget pour le bloc gameplay dans le prompt
)

// BuildContext construit le texte de contexte envoyé au LLM.
func BuildContext(game *models.Game, sections []models.ScoredSection) string {
	if len(sections) == 0 {
		return ""
	}

	var sb strings.Builder

	// En-tête
	sb.WriteString(fmt.Sprintf("=== RÈGLES DU JEU : %s ===\n\n", strings.ToUpper(game.Name)))

	// Métadonnées du jeu (stockées dans game.Metadata)
	if v, ok := game.Metadata["description"].(string); ok && v != "" {
		sb.WriteString(fmt.Sprintf("**Description** : %s\n", v))
	}
	if v, ok := game.Metadata["players"].(string); ok && v != "" {
		sb.WriteString(fmt.Sprintf("**Joueurs** : %s\n", v))
	}
	if v, ok := game.Metadata["duration"].(string); ok && v != "" {
		sb.WriteString(fmt.Sprintf("**Durée** : %s\n", v))
	}
	if v, ok := game.Metadata["complexity"].(string); ok && v != "" {
		sb.WriteString(fmt.Sprintf("**Complexité** : %s\n", v))
	}
	sb.WriteString("\n")

	// Résumé structural du gameplay (mécaniques, phases, setup, tour)
	if gp := buildGameplayContext(game.Gameplay); gp != "" {
		sb.WriteString(gp)
		sb.WriteString("\n")
	}

	// Sections de contexte
	count := maxSections
	if len(sections) < count {
		count = len(sections)
	}

	totalChars := sb.Len()
	for i := 0; i < count; i++ {
		s := sections[i]
		text := strings.TrimSpace(s.Text)
		if text == "" {
			continue
		}

		// Titre de section depuis les métadonnées
		sectionTitle := getSectionTitle(s, i)
		entry := fmt.Sprintf("--- %s ---\n%s\n\n", sectionTitle, text)

		if totalChars+len(entry) > maxContextChars {
			// Tronquer si trop long
			remaining := maxContextChars - totalChars - len(sectionTitle) - 10
			if remaining > 100 {
				entry = fmt.Sprintf("--- %s ---\n%s...\n\n", sectionTitle, text[:remaining])
				sb.WriteString(entry)
			}
			break
		}

		sb.WriteString(entry)
		totalChars += len(entry)
	}

	return strings.TrimSpace(sb.String())
}

// buildGameplayContext génère un bloc textuel compact depuis game.Gameplay.
// Inclut : mécaniques (labels), phases ordonnées, description setup/tour (tronquée).
func buildGameplayContext(gameplay map[string]interface{}) string {
	if len(gameplay) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("--- Gameplay ---\n")

	// Mécaniques
	if mechs, ok := gameplay["mechanics"].([]interface{}); ok && len(mechs) > 0 {
		var labels []string
		for _, m := range mechs {
			if mm, ok := m.(map[string]interface{}); ok {
				if l, ok := mm["label"].(string); ok && l != "" {
					labels = append(labels, l)
				}
			}
		}
		if len(labels) > 0 {
			sb.WriteString(fmt.Sprintf("Mécaniques : %s\n", strings.Join(labels, ", ")))
		}
	}

	// Phases de jeu
	if phases, ok := gameplay["phases"].([]interface{}); ok && len(phases) > 0 {
		sb.WriteString("Phases : ")
		var pnames []string
		for _, p := range phases {
			if pp, ok := p.(map[string]interface{}); ok {
				if n, ok := pp["name"].(string); ok && n != "" {
					pnames = append(pnames, n)
				}
			}
		}
		sb.WriteString(strings.Join(pnames, " → "))
		sb.WriteString("\n")
	}

	// Description setup (tronquée)
	for _, key := range []string{"setup", "turns"} {
		labels := map[string]string{"setup": "Mise en place", "turns": "Tour de jeu"}
		if section, ok := gameplay[key].(map[string]interface{}); ok {
			if desc, ok := section["description"].(string); ok && desc != "" {
				runes := []rune(strings.TrimSpace(desc))
				if len(runes) > 180 {
					desc = string(runes[:180]) + "…"
				}
				sb.WriteString(fmt.Sprintf("%s : %s\n", labels[key], desc))
			}
		}
	}

	result := strings.TrimSpace(sb.String())
	if result == "--- Gameplay ---" {
		return "" // rien d'utile extrait
	}
	if runes := []rune(result); len(runes) > maxGameplayChars {
		result = string(runes[:maxGameplayChars]) + "…"
	}
	return result + "\n"
}

// BuildCompactContext construit un contexte plus court (pour questions simples).
func BuildCompactContext(game *models.Game, sections []models.ScoredSection) string {
	if len(sections) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Jeu : %s\n\n", game.Name))

	maxChars := 1800
	totalChars := 0
	limit := 3
	if len(sections) < limit {
		limit = len(sections)
	}

	for i := 0; i < limit; i++ {
		text := strings.TrimSpace(sections[i].Text)
		if totalChars+len(text) > maxChars {
			break
		}
		sb.WriteString(text)
		sb.WriteString("\n\n")
		totalChars += len(text) + 2
	}

	return strings.TrimSpace(sb.String())
}

var sectionTypeLabels = map[string]string{
	"setup":     "Mise en place",
	"turn":      "Tour de jeu",
	"scoring":   "Score & Victoire",
	"end":       "Fin de partie",
	"component": "Composants",
	"special":   "Règle spéciale",
	"example":   "Exemple",
	"general":   "Règles",
}

func getSectionTitle(s models.ScoredSection, index int) string {
	typeLabel := sectionTypeLabels[s.SectionType]
	if typeLabel == "" {
		typeLabel = "Règles"
	}

	// Titre explicite de la section (ex: "Combat", "Mise en place du plateau")
	if s.Title != "" {
		if s.PageStart != nil {
			return fmt.Sprintf("%s — %s (p.%d)", typeLabel, s.Title, *s.PageStart)
		}
		return fmt.Sprintf("%s — %s", typeLabel, s.Title)
	}

	// Pas de titre : type + page
	if s.PageStart != nil {
		return fmt.Sprintf("%s (p.%d)", typeLabel, *s.PageStart)
	}

	return fmt.Sprintf("%s %d", typeLabel, index+1)
}
