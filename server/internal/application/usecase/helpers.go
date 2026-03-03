// application/usecase/helpers.go — Fonctions utilitaires partagées
package usecase

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
)

// generateGameID génère un ID unique pour un jeu.
func generateGameID(name string) string {
	b := make([]byte, 8)
	rand.Read(b)
	return slugify(name) + "-" + hex.EncodeToString(b)
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
