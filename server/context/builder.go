// context/builder.go — Construction du contexte LLM
package context

import (
	"fmt"
	"strings"

	"ask-rules-server/models"
)

const (
	maxContextChars = 3200
	maxSections     = 6
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

func getSectionTitle(s models.ScoredSection, index int) string {
	if s.Metadata != nil {
		if st, ok := s.Metadata["section_type"].(string); ok && st != "" && st != "general" {
			labels := map[string]string{
				"setup":     "Mise en place",
				"turn":      "Tour de jeu",
				"scoring":   "Score & Victoire",
				"end":       "Fin de partie",
				"component": "Composants",
				"special":   "Règle spéciale",
				"example":   "Exemple",
			}
			if label, ok := labels[st]; ok {
				return label
			}
		}
		if page, ok := s.Metadata["page"].(float64); ok {
			return fmt.Sprintf("Règles (p.%d)", int(page))
		}
	}
	return fmt.Sprintf("Extrait %d", index+1)
}
