// index/slug.go — Slug des jeux (ID et dossier des fichiers)
package index

import (
	"strings"
)

// generateGameID génère l'ID d'un jeu à partir de son nom (slug stable et lisible).
func generateGameID(name string) string {
	return Slugify(name)
}

// Slugify convertit un texte en slug (aussi le dossier des fichiers du jeu).
func Slugify(s string) string {
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
