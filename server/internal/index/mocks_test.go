// index/mocks_test.go — Faux pour les tests du paquet index
package index_test

import (
	"context"
	"slices"

	"ask-rules-server/internal/index"
	"ask-rules-server/internal/store"
)

// Mock GameRepository
type mockGameRepo struct {
	findByIDFunc       func(ctx context.Context, id string) (*store.Game, error)
	findByNameFunc     func(ctx context.Context, name string) (*store.Game, error)
	listFunc           func(ctx context.Context) ([]*store.GameWithStats, error)
	saveFunc           func(ctx context.Context, game *store.Game) error
	deleteFunc         func(ctx context.Context, id string) error
	updateGameplayFunc func(ctx context.Context, id string, gameplay map[string]interface{}) error
	touched            []string
}

func (m *mockGameRepo) FindByID(ctx context.Context, id string) (*store.Game, error) {
	if m.findByIDFunc != nil {
		return m.findByIDFunc(ctx, id)
	}
	return nil, store.ErrGameNotFound
}

func (m *mockGameRepo) FindByName(ctx context.Context, name string) (*store.Game, error) {
	if m.findByNameFunc != nil {
		return m.findByNameFunc(ctx, name)
	}
	return nil, store.ErrGameNotFound
}

func (m *mockGameRepo) List(ctx context.Context) ([]*store.GameWithStats, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx)
	}
	return []*store.GameWithStats{}, nil
}

func (m *mockGameRepo) Save(ctx context.Context, game *store.Game) error {
	if m.saveFunc != nil {
		return m.saveFunc(ctx, game)
	}
	return nil
}

func (m *mockGameRepo) Delete(ctx context.Context, id string) error {
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, id)
	}
	return nil
}

func (m *mockGameRepo) UpdateGameplay(ctx context.Context, id string, gameplay map[string]interface{}) error {
	if m.updateGameplayFunc != nil {
		return m.updateGameplayFunc(ctx, id, gameplay)
	}
	return nil
}

func (m *mockGameRepo) Touch(ctx context.Context, id string) error {
	m.touched = append(m.touched, id)
	return nil
}

// Mock SectionRepository
type mockSectionRepo struct {
	insertFunc         func(ctx context.Context, section *store.Section) error
	deleteByGameIDFunc func(ctx context.Context, gameID string) error
	vectorSearchFunc   func(ctx context.Context, gameID string, embedding []float64, limit int) ([]*store.ScoredSection, error)
	fullTextSearchFunc func(ctx context.Context, gameID, query string, limit int) ([]*store.ScoredSection, error)
	// ids : IDs des sections présentes (ListIDsByGameID, DeleteByIDs), tous jeux confondus
	ids []string
}

func (m *mockSectionRepo) Insert(ctx context.Context, section *store.Section) error {
	if m.insertFunc != nil {
		return m.insertFunc(ctx, section)
	}
	return nil
}

func (m *mockSectionRepo) DeleteByGameID(ctx context.Context, gameID string) error {
	if m.deleteByGameIDFunc != nil {
		return m.deleteByGameIDFunc(ctx, gameID)
	}
	return nil
}

func (m *mockSectionRepo) ListIDsByGameID(ctx context.Context, gameID string) ([]string, error) {
	return slices.Clone(m.ids), nil
}

func (m *mockSectionRepo) DeleteByIDs(ctx context.Context, ids []string) error {
	m.ids = slices.DeleteFunc(m.ids, func(id string) bool { return slices.Contains(ids, id) })
	return nil
}

func (m *mockSectionRepo) VectorSearch(ctx context.Context, gameID string, embedding []float64, limit int) ([]*store.ScoredSection, error) {
	if m.vectorSearchFunc != nil {
		return m.vectorSearchFunc(ctx, gameID, embedding, limit)
	}
	return []*store.ScoredSection{}, nil
}

func (m *mockSectionRepo) FullTextSearch(ctx context.Context, gameID, query string, limit int) ([]*store.ScoredSection, error) {
	if m.fullTextSearchFunc != nil {
		return m.fullTextSearchFunc(ctx, gameID, query, limit)
	}
	return []*store.ScoredSection{}, nil
}

func (m *mockSectionRepo) Neighbors(ctx context.Context, sec *store.ScoredSection) ([]*store.ScoredSection, error) {
	return []*store.ScoredSection{sec}, nil
}

// Mock Processor
type mockPipeline struct {
	processFunc func(ctx context.Context, opts *index.ImportOptions) error
}

func (m *mockPipeline) Process(ctx context.Context, opts *index.ImportOptions) error {
	if m.processFunc != nil {
		return m.processFunc(ctx, opts)
	}
	return nil
}
