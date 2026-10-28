// application/usecase/ask_question_test.go — Tests unitaires du use case
package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"ask-rules-server/internal/application/usecase"
	"ask-rules-server/internal/domain/entity"
	"ask-rules-server/internal/domain/service"
)

// Test : Cas nominal
func TestAskQuestionUseCase_Execute_Success(t *testing.T) {
	// Arrange
	gameRepo := &mockGameRepo{
		findByNameFunc: func(ctx context.Context, name string) (*entity.Game, error) {
			return &entity.Game{
				ID:   "game-1",
				Name: "Wingspan",
			}, nil
		},
	}

	retriever := &mockRetriever{
		searchFunc: func(ctx context.Context, req *service.SearchRequest) ([]*entity.ScoredSection, error) {
			pageStart := 2
			return []*entity.ScoredSection{
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

	uc := usecase.NewAskQuestionUseCase(gameRepo, retriever, llmSvc, cache, logRepo)

	req := &usecase.AskRequest{
		GameName: "Wingspan",
		Question: "How do I setup the game?",
	}

	// Act
	response, err := uc.Execute(context.Background(), req)

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
func TestAskQuestionUseCase_Execute_GameNotFound(t *testing.T) {
	// Arrange
	gameRepo := &mockGameRepo{} // Retourne ErrGameNotFound par défaut
	retriever := &mockRetriever{}
	llmSvc := &mockLLM{}
	cache := newMockCache()
	logRepo := &mockLogRepo{}

	uc := usecase.NewAskQuestionUseCase(gameRepo, retriever, llmSvc, cache, logRepo)

	req := &usecase.AskRequest{
		GameName: "Unknown Game",
		Question: "How to play?",
	}

	// Act
	_, err := uc.Execute(context.Background(), req)

	// Assert
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	if !errors.Is(err, entity.ErrGameNotFound) {
		t.Errorf("Expected ErrGameNotFound, got: %v", err)
	}
}

// Test : Cache hit
func TestAskQuestionUseCase_Execute_CacheHit(t *testing.T) {
	// Arrange
	gameRepo := &mockGameRepo{
		findByNameFunc: func(ctx context.Context, name string) (*entity.Game, error) {
			return &entity.Game{ID: "game-1", Name: "Wingspan"}, nil
		},
	}

	retriever := &mockRetriever{
		searchFunc: func(ctx context.Context, req *service.SearchRequest) ([]*entity.ScoredSection, error) {
			pageStart := 2
			return []*entity.ScoredSection{
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

	uc := usecase.NewAskQuestionUseCase(gameRepo, retriever, llmSvc, cache, logRepo)

	req := &usecase.AskRequest{
		GameName: "Wingspan",
		Question: "How to play?",
	}

	// First call (cache miss)
	resp1, _ := uc.Execute(context.Background(), req)

	// Second call (cache hit)
	resp2, err := uc.Execute(context.Background(), req)

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
func TestAskQuestionUseCase_CacheInvalidatedByReindex(t *testing.T) {
	game := &entity.Game{ID: "game-1", Name: "Wingspan", UpdatedAt: time.Unix(1000, 0)}
	gameRepo := &mockGameRepo{
		findByNameFunc: func(ctx context.Context, name string) (*entity.Game, error) {
			g := *game
			return &g, nil
		},
	}
	retriever := &mockRetriever{
		searchFunc: func(ctx context.Context, req *service.SearchRequest) ([]*entity.ScoredSection, error) {
			return []*entity.ScoredSection{{ID: "s1", GameID: "game-1", Title: "Setup", Text: "..."}}, nil
		},
	}
	uc := usecase.NewAskQuestionUseCase(gameRepo, retriever, &mockLLM{}, newMockCache(), &mockLogRepo{})
	ask := func() *usecase.AskResponse {
		resp, err := uc.Execute(context.Background(), &usecase.AskRequest{GameName: "Wingspan", Question: "Setup ?"})
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
