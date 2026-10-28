// ask/ask_test.go — Tests de Service.Execute
package ask_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"ask-rules-server/internal/ask"
	"ask-rules-server/internal/store"
)

// Test : Cas nominal
func TestAsk_Execute_Success(t *testing.T) {
	// Arrange
	gameRepo := &mockGameRepo{
		findByNameFunc: func(ctx context.Context, name string) (*store.Game, error) {
			return &store.Game{
				ID:   "game-1",
				Name: "Wingspan",
			}, nil
		},
	}

	retriever := &mockRetriever{
		searchFunc: func(ctx context.Context, gameID, question string, limit int) ([]*store.ScoredSection, error) {
			pageStart := 2
			return []*store.ScoredSection{
				{
					ID:        "section-1",
					GameID:    "game-1",
					Title:     "Setup",
					Text:      "Place the board in the center...",
					PageStart: &pageStart,
					Score:     0.92,
				},
			}, nil
		},
	}

	llmSvc := &mockLLM{}
	cache := newMockCache()
	logRepo := &mockLogRepo{}

	svc := ask.NewService(gameRepo, retriever, llmSvc, cache, logRepo)

	req := &ask.Request{
		GameName: "Wingspan",
		Question: "How do I setup the game?",
	}

	// Act
	response, err := svc.Execute(context.Background(), req)

	// Assert
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if response.Game != "Wingspan" {
		t.Errorf("Expected game 'Wingspan', got '%s'", response.Game)
	}

	if response.Answer != "Mocked answer" {
		t.Errorf("Expected answer 'Mocked answer', got '%s'", response.Answer)
	}

	if len(response.Sections) != 1 {
		t.Errorf("Expected 1 section, got %d", len(response.Sections))
	}

	if response.Cached {
		t.Error("Expected cached=false on first call")
	}
}

// Test : Jeu non trouvé
func TestAsk_Execute_GameNotFound(t *testing.T) {
	// Arrange
	gameRepo := &mockGameRepo{} // Retourne ErrGameNotFound par défaut
	retriever := &mockRetriever{}
	llmSvc := &mockLLM{}
	cache := newMockCache()
	logRepo := &mockLogRepo{}

	svc := ask.NewService(gameRepo, retriever, llmSvc, cache, logRepo)

	req := &ask.Request{
		GameName: "Unknown Game",
		Question: "How to play?",
	}

	// Act
	_, err := svc.Execute(context.Background(), req)

	// Assert
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	if !errors.Is(err, store.ErrGameNotFound) {
		t.Errorf("Expected ErrGameNotFound, got: %v", err)
	}
}

// Test : Cache hit
func TestAsk_Execute_CacheHit(t *testing.T) {
	// Arrange
	gameRepo := &mockGameRepo{
		findByNameFunc: func(ctx context.Context, name string) (*store.Game, error) {
			return &store.Game{ID: "game-1", Name: "Wingspan"}, nil
		},
	}

	retriever := &mockRetriever{
		searchFunc: func(ctx context.Context, gameID, question string, limit int) ([]*store.ScoredSection, error) {
			pageStart := 2
			return []*store.ScoredSection{
				{
					ID:        "section-1",
					GameID:    "game-1",
					Title:     "Setup",
					Text:      "Place the board...",
					PageStart: &pageStart,
					Score:     0.92,
				},
			}, nil
		},
	}
	llmSvc := &mockLLM{}
	cache := newMockCache()
	logRepo := &mockLogRepo{}

	svc := ask.NewService(gameRepo, retriever, llmSvc, cache, logRepo)

	req := &ask.Request{
		GameName: "Wingspan",
		Question: "How to play?",
	}

	// First call (cache miss)
	resp1, _ := svc.Execute(context.Background(), req)

	// Second call (cache hit)
	resp2, err := svc.Execute(context.Background(), req)

	// Assert
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if !resp2.Cached {
		t.Error("Expected cached=true on second call")
	}

	if resp1.Answer != resp2.Answer {
		t.Error("Cached response should match original")
	}
}

// Test : une réindexation (date_update modifiée) invalide les réponses en cache
func TestAsk_CacheInvalidatedByReindex(t *testing.T) {
	game := &store.Game{ID: "game-1", Name: "Wingspan", UpdatedAt: time.Unix(1000, 0)}
	gameRepo := &mockGameRepo{
		findByNameFunc: func(ctx context.Context, name string) (*store.Game, error) {
			g := *game
			return &g, nil
		},
	}
	retriever := &mockRetriever{
		searchFunc: func(ctx context.Context, gameID, question string, limit int) ([]*store.ScoredSection, error) {
			return []*store.ScoredSection{{ID: "s1", GameID: "game-1", Title: "Setup", Text: "..."}}, nil
		},
	}
	svc := ask.NewService(gameRepo, retriever, &mockLLM{}, newMockCache(), &mockLogRepo{})
	ask := func() *ask.Response {
		resp, err := svc.Execute(context.Background(), &ask.Request{GameName: "Wingspan", Question: "Setup ?"})
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		return resp
	}

	ask()
	if !ask().Cached {
		t.Fatal("la deuxième question identique doit venir du cache")
	}
	game.UpdatedAt = time.Unix(2000, 0)
	if ask().Cached {
		t.Error("après réindexation, la réponse ne doit plus venir du cache")
	}
}
