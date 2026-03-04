// domain/repository/section_repository.go — Interface repository pour les sections
package repository

import (
	"context"

	"ask-rules-server/internal/domain/entity"
)

// SectionRepository définit les opérations sur les sections de règles.
type SectionRepository interface {
	// Insert insère une nouvelle section dans la base.
	Insert(ctx context.Context, section *entity.Section) error

	// DeleteByGameID supprime toutes les sections d'un jeu.
	DeleteByGameID(ctx context.Context, gameID string) error

	// VectorSearch effectue une recherche sémantique par embedding.
	// Retourne les sections les plus similaires triées par score décroissant.
	VectorSearch(ctx context.Context, gameID string, embedding []float64, limit int) ([]*entity.ScoredSection, error)

	// FullTextSearch effectue une recherche full-text (PostgreSQL tsvector).
	// Retourne les sections correspondantes triées par pertinence.
	FullTextSearch(ctx context.Context, gameID string, query string, limit int) ([]*entity.ScoredSection, error)
}
