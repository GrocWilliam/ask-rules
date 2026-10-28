// store/errors.go — Erreurs sur les jeux
package store

import (
	"errors"
)

var (
	// ErrGameNotFound est retourné quand un jeu n'existe pas.
	ErrGameNotFound = errors.New("game not found")

	// ErrInvalidGameName est retourné quand le nom du jeu est invalide.
	ErrInvalidGameName = errors.New("invalid game name")
)
