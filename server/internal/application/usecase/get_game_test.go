// application/usecase/get_game_test.go — Tests unitaires du use case GetGame
package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"ask-rules-server/internal/application/usecase"
	"ask-rules-server/internal/domain/entity"
)

// Test : Cas nominal - Récupérer un jeu existant
func TestGetGameUseCase_Execute_Success(t *testing.T) {
	// Arrange
	now := time.Now()
	gameRepo := &mockGameRepo{
		findByIDFunc: func(ctx context.Context, id string) (*entity.Game, error) {
			if id == "game-1" {
				return &entity.Game{
					ID:      "game-1",
					Name:    "Wingspan",
					AddedAt: now.Add(-24 * time.Hour),
					Gameplay: map[string]interface{}{
						"players": "1-5",
						"age":     "10+",
					},
				}, nil
			}
			return nil, entity.ErrGameNotFound
		},
	}

	uc := usecase.NewGetGameUseCase(gameRepo)

	// Act
	game, err := uc.Execute(context.Background(), "game-1")

	// Assert
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if game.ID != "game-1" {
		t.Errorf("Expected ID 'game-1', got '%s'", game.ID)
	}

	if game.Name != "Wingspan" {
		t.Errorf("Expected name 'Wingspan', got '%s'", game.Name)
	}

	if game.Gameplay == nil {
		t.Fatal("Expected gameplay data, got nil")
	}

	if game.Gameplay["players"] != "1-5" {
		t.Errorf("Expected players '1-5', got '%v'", game.Gameplay["players"])
	}
}

// Test : ID vide
func TestGetGameUseCase_Execute_EmptyID(t *testing.T) {
	// Arrange
	gameRepo := &mockGameRepo{}
	uc := usecase.NewGetGameUseCase(gameRepo)

	// Act
	game, err := uc.Execute(context.Background(), "")

	// Assert
	if err == nil {
		t.Fatal("Expected error for empty ID, got nil")
	}

	if game != nil {
		t.Errorf("Expected nil game on error, got: %+v", game)
	}
}

// Test : Jeu non trouvé
func TestGetGameUseCase_Execute_GameNotFound(t *testing.T) {
	// Arrange
	gameRepo := &mockGameRepo{
		findByIDFunc: func(ctx context.Context, id string) (*entity.Game, error) {
			return nil, entity.ErrGameNotFound
		},
	}

	uc := usecase.NewGetGameUseCase(gameRepo)

	// Act
	game, err := uc.Execute(context.Background(), "nonexistent")

	// Assert
	if err == nil {
		t.Fatal("Expected error for nonexistent game, got nil")
	}

	if !errors.Is(err, entity.ErrGameNotFound) {
		t.Errorf("Expected ErrGameNotFound, got: %v", err)
	}

	if game != nil {
		t.Errorf("Expected nil game on error, got: %+v", game)
	}
}

// Test : Erreur du repository
func TestGetGameUseCase_Execute_RepositoryError(t *testing.T) {
	// Arrange
	expectedErr := errors.New("database error")
	gameRepo := &mockGameRepo{
		findByIDFunc: func(ctx context.Context, id string) (*entity.Game, error) {
			return nil, expectedErr
		},
	}

	uc := usecase.NewGetGameUseCase(gameRepo)

	// Act
	game, err := uc.Execute(context.Background(), "game-1")

	// Assert
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	if game != nil {
		t.Errorf("Expected nil game on error, got: %+v", game)
	}
}
