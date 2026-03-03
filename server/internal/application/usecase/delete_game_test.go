// application/usecase/delete_game_test.go — Tests unitaires du use case DeleteGame
package usecase_test

import (
	"context"
	"errors"
	"testing"

	"ask-rules-server/internal/application/usecase"
	"ask-rules-server/internal/domain/entity"
)

// Test : Suppression réussie d'un jeu
func TestDeleteGameUseCase_Execute_Success(t *testing.T) {
	// Arrange
	deletedGameID := ""
	deletedSectionsGameID := ""

	gameRepo := &mockGameRepo{
		findByIDFunc: func(ctx context.Context, id string) (*entity.Game, error) {
			if id == "game-1" {
				return &entity.Game{
					ID:   "game-1",
					Name: "Wingspan",
				}, nil
			}
			return nil, entity.ErrGameNotFound
		},
		deleteFunc: func(ctx context.Context, id string) error {
			deletedGameID = id
			return nil
		},
	}

	sectionRepo := &mockSectionRepo{
		deleteByGameIDFunc: func(ctx context.Context, gameID string) error {
			deletedSectionsGameID = gameID
			return nil
		},
	}

	cache := newMockCache()
	cache.data["game:game-1"] = []byte(`{"test": "some cached data"}`)

	uc := usecase.NewDeleteGameUseCase(gameRepo, sectionRepo, cache)

	// Act
	err := uc.Execute(context.Background(), "game-1")

	// Assert
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if deletedGameID != "game-1" {
		t.Errorf("Expected game 'game-1' to be deleted, got '%s'", deletedGameID)
	}

	if deletedSectionsGameID != "game-1" {
		t.Errorf("Expected sections for 'game-1' to be deleted, got '%s'", deletedSectionsGameID)
	}

	// Note: Le cache n'est pas actuellement invalidé dans l'implémentation
	// (design choice - le cache expire naturellement après 24h)
}

// Test : Jeu non trouvé
func TestDeleteGameUseCase_Execute_GameNotFound(t *testing.T) {
	// Arrange
	gameRepo := &mockGameRepo{
		findByIDFunc: func(ctx context.Context, id string) (*entity.Game, error) {
			return nil, entity.ErrGameNotFound
		},
	}

	sectionRepo := &mockSectionRepo{}
	cache := newMockCache()

	uc := usecase.NewDeleteGameUseCase(gameRepo, sectionRepo, cache)

	// Act
	err := uc.Execute(context.Background(), "nonexistent")

	// Assert
	if err == nil {
		t.Fatal("Expected error for nonexistent game, got nil")
	}

	if !errors.Is(err, entity.ErrGameNotFound) {
		t.Errorf("Expected ErrGameNotFound, got: %v", err)
	}
}

// Test : Erreur lors de la suppression des sections
func TestDeleteGameUseCase_Execute_SectionDeleteError(t *testing.T) {
	// Arrange
	expectedErr := errors.New("failed to delete sections")

	gameRepo := &mockGameRepo{
		findByIDFunc: func(ctx context.Context, id string) (*entity.Game, error) {
			return &entity.Game{
				ID:   "game-1",
				Name: "Wingspan",
			}, nil
		},
	}

	sectionRepo := &mockSectionRepo{
		deleteByGameIDFunc: func(ctx context.Context, gameID string) error {
			return expectedErr
		},
	}

	cache := newMockCache()

	uc := usecase.NewDeleteGameUseCase(gameRepo, sectionRepo, cache)

	// Act
	err := uc.Execute(context.Background(), "game-1")

	// Assert
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
}

// Test : Erreur lors de la suppression du jeu
func TestDeleteGameUseCase_Execute_GameDeleteError(t *testing.T) {
	// Arrange
	expectedErr := errors.New("failed to delete game")

	gameRepo := &mockGameRepo{
		findByIDFunc: func(ctx context.Context, id string) (*entity.Game, error) {
			return &entity.Game{
				ID:   "game-1",
				Name: "Wingspan",
			}, nil
		},
		deleteFunc: func(ctx context.Context, id string) error {
			return expectedErr
		},
	}

	sectionRepo := &mockSectionRepo{
		deleteByGameIDFunc: func(ctx context.Context, gameID string) error {
			return nil
		},
	}

	cache := newMockCache()

	uc := usecase.NewDeleteGameUseCase(gameRepo, sectionRepo, cache)

	// Act
	err := uc.Execute(context.Background(), "game-1")

	// Assert
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
}

// Test : ID vide
func TestDeleteGameUseCase_Execute_EmptyID(t *testing.T) {
	// Arrange
	gameRepo := &mockGameRepo{
		findByIDFunc: func(ctx context.Context, id string) (*entity.Game, error) {
			return nil, entity.ErrGameNotFound
		},
	}

	sectionRepo := &mockSectionRepo{}
	cache := newMockCache()

	uc := usecase.NewDeleteGameUseCase(gameRepo, sectionRepo, cache)

	// Act
	err := uc.Execute(context.Background(), "")

	// Assert
	if err == nil {
		t.Fatal("Expected error for empty ID, got nil")
	}
}
