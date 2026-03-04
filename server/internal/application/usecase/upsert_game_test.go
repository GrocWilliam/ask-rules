// application/usecase/upsert_game_test.go — Tests unitaires du use case UpsertGame
package usecase_test

import (
	"context"
	"errors"
	"testing"

	"ask-rules-server/internal/application/usecase"
	"ask-rules-server/internal/domain/entity"
)

// Test : Création d'un nouveau jeu
func TestUpsertGameUseCase_Execute_CreateNew(t *testing.T) {
	// Arrange
	var savedGame *entity.Game
	gameRepo := &mockGameRepo{
		saveFunc: func(ctx context.Context, game *entity.Game) error {
			savedGame = game
			return nil
		},
		findByIDFunc: func(ctx context.Context, id string) (*entity.Game, error) {
			return nil, entity.ErrGameNotFound
		},
	}

	uc := usecase.NewUpsertGameUseCase(gameRepo)

	req := &usecase.UpsertGameRequest{
		Name: "Spirit Island",
		Gameplay: map[string]interface{}{
			"players": "1-4",
			"age":     "13+",
		},
	}

	// Act
	result, err := uc.Execute(context.Background(), req)

	// Assert
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if result.Name != "Spirit Island" {
		t.Errorf("Expected name 'Spirit Island', got '%s'", result.Name)
	}

	if result.ID == "" {
		t.Error("Expected generated ID, got empty string")
	}

	if savedGame == nil {
		t.Fatal("Expected game to be saved")
	}

	if savedGame.Name != "Spirit Island" {
		t.Errorf("Expected saved game name 'Spirit Island', got '%s'", savedGame.Name)
	}

	if savedGame.Gameplay["players"] != "1-4" {
		t.Errorf("Expected players '1-4', got '%v'", savedGame.Gameplay["players"])
	}
}

// Test : Mise à jour d'un jeu existant
func TestUpsertGameUseCase_Execute_UpdateExisting(t *testing.T) {
	// Arrange
	var savedGame *entity.Game
	existingGame := &entity.Game{
		ID:   "game-1",
		Name: "Wingspan",
	}

	gameRepo := &mockGameRepo{
		findByIDFunc: func(ctx context.Context, id string) (*entity.Game, error) {
			if id == "game-1" {
				return existingGame, nil
			}
			return nil, entity.ErrGameNotFound
		},
		saveFunc: func(ctx context.Context, game *entity.Game) error {
			savedGame = game
			return nil
		},
	}

	uc := usecase.NewUpsertGameUseCase(gameRepo)

	req := &usecase.UpsertGameRequest{
		ID:   "game-1",
		Name: "Wingspan (Updated)",
		Gameplay: map[string]interface{}{
			"expansion": "European",
		},
	}

	// Act
	result, err := uc.Execute(context.Background(), req)

	// Assert
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if result.ID != "game-1" {
		t.Errorf("Expected ID 'game-1', got '%s'", result.ID)
	}

	if result.Name != "Wingspan (Updated)" {
		t.Errorf("Expected updated name, got '%s'", result.Name)
	}

	if savedGame == nil {
		t.Fatal("Expected game to be saved")
	}

	if savedGame.Gameplay["expansion"] != "European" {
		t.Errorf("Expected expansion 'European', got '%v'", savedGame.Gameplay["expansion"])
	}
}

// Test : Nom vide (erreur de validation)
func TestUpsertGameUseCase_Execute_EmptyName(t *testing.T) {
	// Arrange
	gameRepo := &mockGameRepo{}
	uc := usecase.NewUpsertGameUseCase(gameRepo)

	req := &usecase.UpsertGameRequest{
		Name: "",
	}

	// Act
	result, err := uc.Execute(context.Background(), req)

	// Assert
	if err == nil {
		t.Fatal("Expected error for empty name, got nil")
	}

	if !errors.Is(err, entity.ErrInvalidGameName) {
		t.Errorf("Expected ErrInvalidGameName, got: %v", err)
	}

	if result != nil {
		t.Errorf("Expected nil result on error, got: %+v", result)
	}
}

// Test : Erreur lors de la sauvegarde
func TestUpsertGameUseCase_Execute_SaveError(t *testing.T) {
	// Arrange
	expectedErr := errors.New("database error")
	gameRepo := &mockGameRepo{
		saveFunc: func(ctx context.Context, game *entity.Game) error {
			return expectedErr
		},
		findByIDFunc: func(ctx context.Context, id string) (*entity.Game, error) {
			return nil, entity.ErrGameNotFound
		},
	}

	uc := usecase.NewUpsertGameUseCase(gameRepo)

	req := &usecase.UpsertGameRequest{
		Name: "Test Game",
	}

	// Act
	result, err := uc.Execute(context.Background(), req)

	// Assert
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	if result != nil {
		t.Errorf("Expected nil result on error, got: %+v", result)
	}
}
