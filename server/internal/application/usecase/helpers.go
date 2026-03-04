// application/usecase/helpers.go — Fonctions utilitaires partagées
package usecase

import (
	"strings"
)

// generateGameID génère l'ID d'un jeu à partir de son nom (slug stable et lisible).
func generateGameID(name string) string {
	return slugify(name)
}

// slugify convertit un texte en slug.
func slugify(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "'", "")
	// Enlever les caractères non-alphanumériques sauf tirets
	var result strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// toStringSlice convertit un []interface{} (issu de JSON/pgx) en []string.
func toStringSlice(v interface{}) []string {
	if v == nil {
		return nil
	}
	if s, ok := v.([]string); ok {
		return s
	}
	if raw, ok := v.([]interface{}); ok {
		result := make([]string, 0, len(raw))
		for _, item := range raw {
			if str, ok := item.(string); ok {
				result = append(result, str)
			}
		}
		return result
	}
	return nil
}
