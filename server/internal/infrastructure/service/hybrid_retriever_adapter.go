// infrastructure/service/hybrid_retriever_adapter.go — Adapter pour  retriever hybride
package service

import (
	"context"

	"ask-rules-server/internal/domain/entity"
	"ask-rules-server/internal/domain/repository"
	"ask-rules-server/internal/domain/service"
	"ask-rules-server/internal/domain/service/retriever"
)

// HybridRetrieverAdapter adapte le package retriever existant à l'interface domaine.
type HybridRetrieverAdapter struct {
	embedder    service.EmbedderService
	sectionRepo repository.SectionRepository
}

// NewHybridRetriever crée un nouvel adapter pour le retriever hybride.
func NewHybridRetriever(sectionRepo repository.SectionRepository, embedder service.EmbedderService) service.RetrieverService {
	return &HybridRetrieverAdapter{
		embedder:    embedder,
		sectionRepo: sectionRepo,
	}
}

// Search effectue une recherche hybride pour trouver les sections pertinentes.
func (r *HybridRetrieverAdapter) Search(ctx context.Context, req *service.SearchRequest) ([]*entity.ScoredSection, error) {
	// Appeler le retriever existant
	opts := retriever.SearchOptions{
		GameID:      req.GameID,
		Question:    req.Question,
		Limit:       req.Limit,
		Embedder:    r.embedder,
		SectionRepo: r.sectionRepo,
	}

	results, err := retriever.Search(ctx, opts)
	if err != nil {
		return nil, err
	}

	// Convertir []entity.ScoredSection → []*entity.ScoredSection
	sections := make([]*entity.ScoredSection, len(results))
	for i := range results {
		sections[i] = &results[i]
	}

	return sections, nil
}
