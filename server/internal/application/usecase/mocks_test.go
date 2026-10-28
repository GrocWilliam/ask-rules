// application/usecase/mocks_test.go — Mocks partagés pour les tests
package usecase_test

import (
	"context"
	"encoding/json"
	"slices"

	"ask-rules-server/internal/application/usecase"
	"ask-rules-server/internal/domain/entity"
	"ask-rules-server/internal/domain/service"
)

// Mock GameRepository
type mockGameRepo struct {
	findByIDFunc       func(ctx context.Context, id string) (*entity.Game, error)
	findByNameFunc     func(ctx context.Context, name string) (*entity.Game, error)
	listFunc           func(ctx context.Context) ([]*entity.GameWithStats, error)
	saveFunc           func(ctx context.Context, game *entity.Game) error
	deleteFunc         func(ctx context.Context, id string) error
	updateGameplayFunc func(ctx context.Context, id string, gameplay map[string]interface{}) error
	touched            []string
}

func (m *mockGameRepo) FindByID(ctx context.Context, id string) (*entity.Game, error) {
	if m.findByIDFunc != nil {
		return m.findByIDFunc(ctx, id)
	}
	return nil, entity.ErrGameNotFound
}

func (m *mockGameRepo) FindByName(ctx context.Context, name string) (*entity.Game, error) {
	if m.findByNameFunc != nil {
		return m.findByNameFunc(ctx, name)
	}
	return nil, entity.ErrGameNotFound
}

func (m *mockGameRepo) List(ctx context.Context) ([]*entity.GameWithStats, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx)
	}
	return []*entity.GameWithStats{}, nil
}

func (m *mockGameRepo) Save(ctx context.Context, game *entity.Game) error {
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
	insertFunc         func(ctx context.Context, section *entity.Section) error
	deleteByGameIDFunc func(ctx context.Context, gameID string) error
	vectorSearchFunc   func(ctx context.Context, gameID string, embedding []float64, limit int) ([]*entity.ScoredSection, error)
	fullTextSearchFunc func(ctx context.Context, gameID, query string, limit int) ([]*entity.ScoredSection, error)
	// ids : IDs des sections présentes (ListIDsByGameID, DeleteByIDs), tous jeux confondus
	ids []string
}

func (m *mockSectionRepo) Insert(ctx context.Context, section *entity.Section) error {
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

func (m *mockSectionRepo) VectorSearch(ctx context.Context, gameID string, embedding []float64, limit int) ([]*entity.ScoredSection, error) {
	if m.vectorSearchFunc != nil {
		return m.vectorSearchFunc(ctx, gameID, embedding, limit)
	}
	return []*entity.ScoredSection{}, nil
}

func (m *mockSectionRepo) FullTextSearch(ctx context.Context, gameID, query string, limit int) ([]*entity.ScoredSection, error) {
	if m.fullTextSearchFunc != nil {
		return m.fullTextSearchFunc(ctx, gameID, query, limit)
	}
	return []*entity.ScoredSection{}, nil
}

func (m *mockSectionRepo) Neighbors(ctx context.Context, sec *entity.ScoredSection) ([]*entity.ScoredSection, error) {
	return []*entity.ScoredSection{sec}, nil
}

// Mock RetrieverService
type mockRetriever struct {
	searchFunc func(ctx context.Context, req *service.SearchRequest) ([]*entity.ScoredSection, error)
}

func (m *mockRetriever) Search(ctx context.Context, req *service.SearchRequest) ([]*entity.ScoredSection, error) {
	if m.searchFunc != nil {
		return m.searchFunc(ctx, req)
	}
	return []*entity.ScoredSection{}, nil
}

// Mock LLMService
type mockLLM struct {
	queryFunc func(ctx context.Context, question, context string, history []entity.ChatTurn, onToken func(string)) (*service.LLMResponse, error)
}

func (m *mockLLM) Query(ctx context.Context, question, context string, history []entity.ChatTurn, onToken func(string)) (*service.LLMResponse, error) {
	if m.queryFunc != nil {
		return m.queryFunc(ctx, question, context, history, onToken)
	}
	if onToken != nil {
		onToken("Mocked ")
		onToken("answer")
	}
	return &service.LLMResponse{
		Answer: "Mocked answer",
		Model:  "mock-model",
	}, nil
}

func (m *mockLLM) Warmup() {}

// Mock CacheService
type mockCache struct {
	data map[string][]byte // Stocker comme bytes pour simuler JSON
}

func newMockCache() *mockCache {
	return &mockCache{data: make(map[string][]byte)}
}

func (m *mockCache) Get(ctx context.Context, key string) (interface{}, error) {
	if val, ok := m.data[key]; ok {
		// Désérialiser en map comme le vrai cache
		var result map[string]interface{}
		if err := json.Unmarshal(val, &result); err != nil {
			return nil, err
		}
		return result, nil
	}
	return nil, nil // Cache miss sans erreur
}

func (m *mockCache) Set(ctx context.Context, key string, value interface{}) error {
	// Sérialiser comme le vrai cache
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	m.data[key] = b
	return nil
}

// Mock PipelineService
type mockPipeline struct {
	processFunc func(ctx context.Context, opts *usecase.ImportOptions) error
}

func (m *mockPipeline) Process(ctx context.Context, opts *usecase.ImportOptions) error {
	if m.processFunc != nil {
		return m.processFunc(ctx, opts)
	}
	return nil
}
