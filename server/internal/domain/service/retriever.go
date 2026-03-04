// domain/service/retriever.go — Interface service pour la recherche hybride
package service

import (
	"context"

	"ask-rules-server/internal/domain/entity"
)

// RetrieverService effectue une recherche hybride (vectorielle + full-text).
type RetrieverService interface {
	// Search effectue une recherche hybride pour trouver les sections pertinentes.
	// Combine recherche vectorielle (sémantique) et full-text (mots-clés).
	Search(ctx context.Context, req *SearchRequest) ([]*entity.ScoredSection, error)
}

// SearchRequest paramètres de recherche.
type SearchRequest struct {
	// GameID est l'identifiant du jeu à rechercher.
	GameID string

	// Question est la question posée par l'utilisateur.
	Question string

	// Limit est le nombre maximum de sections à retourner.
	Limit int
}
