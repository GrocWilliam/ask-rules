// index/service.go — Gestion des jeux : import et indexation des règles,
// réindexation, renommage, suppression
package index

import (
	"context"

	"ask-rules-server/internal/store"
)

// Processor indexe les fichiers d'un jeu (Pipeline ; un faux dans les tests).
type Processor interface {
	Process(ctx context.Context, opts *ImportOptions) error
}

// Service gère les jeux et leurs sections.
type Service struct {
	gameRepo    store.GameRepository
	sectionRepo store.SectionRepository
	pipeline    Processor
	logRepo     store.LogRepository
}

// NewService crée le service ; logRepo peut être nil (pas de journal).
func NewService(gameRepo store.GameRepository, sectionRepo store.SectionRepository, pipeline Processor, logRepo store.LogRepository) *Service {
	return &Service{gameRepo: gameRepo, sectionRepo: sectionRepo, pipeline: pipeline, logRepo: logRepo}
}
