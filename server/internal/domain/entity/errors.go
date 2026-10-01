// domain/entity/errors.go — Erreurs du domaine
package entity

import (
	"errors"
)

var (
	// ErrGameNotFound est retourné quand un jeu n'existe pas.
	ErrGameNotFound = errors.New("game not found")

	// ErrInvalidGameName est retourné quand le nom du jeu est invalide.
	ErrInvalidGameName = errors.New("invalid game name")

	// ErrNoSectionsFound est retourné quand aucune section pertinente n'est trouvée.
	ErrNoSectionsFound = errors.New("no relevant sections found")

	// ErrLLMRateLimited est retourné quand le fournisseur LLM refuse la requête
	// pour dépassement de quota (HTTP 429), même après plusieurs tentatives.
	ErrLLMRateLimited = errors.New("llm rate limited")
)
