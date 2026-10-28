// ask/mocks_test.go — Faux pour les tests du paquet ask
package ask_test

import (
	"context"
	"encoding/json"

	"ask-rules-server/internal/llm"
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

// Mock Searcher
type mockRetriever struct {
	searchFunc func(ctx context.Context, gameID, question string, limit int) ([]*store.ScoredSection, error)
}

func (m *mockRetriever) Search(ctx context.Context, gameID, question string, limit int) ([]*store.ScoredSection, error) {
	if m.searchFunc != nil {
		return m.searchFunc(ctx, gameID, question, limit)
	}
	return []*store.ScoredSection{}, nil
}

// Mock LLM
type mockLLM struct {
	queryFunc func(ctx context.Context, question, context string, history []llm.ChatTurn, onToken func(string)) (*llm.Response, error)
}

func (m *mockLLM) Query(ctx context.Context, question, context string, history []llm.ChatTurn, onToken func(string)) (*llm.Response, error) {
	if m.queryFunc != nil {
		return m.queryFunc(ctx, question, context, history, onToken)
	}
	if onToken != nil {
		onToken("Mocked ")
		onToken("answer")
	}
	return &llm.Response{
		Answer: "Mocked answer",
		Model:  "mock-model",
	}, nil
}

// Mock Cache
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

// Mock LogRepository
type mockLogRepo struct{}

func (m *mockLogRepo) Save(ctx context.Context, entry *store.LogEntry) error {
	return nil
}

func (m *mockLogRepo) GetRecent(ctx context.Context, limit int) ([]*store.LogEntry, error) {
	return []*store.LogEntry{}, nil
}
