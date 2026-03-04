// application/usecase/list_games_test.go — Tests unitaires du use case ListGames
package usecase_test

import (
	"context"
	"errors"
	"testing"

	"ask-rules-server/internal/application/usecase"
	"ask-rules-server/internal/domain/entity"
)

// Test : Cas nominal - Liste plusieurs jeux
func TestListGamesUseCase_Execute_Success(t *testing.T) {
	// Arrange
	gameRepo := &mockGameRepo{
		listFunc: func(ctx context.Context) ([]*entity.GameWithStats, error) {
			return []*entity.GameWithStats{
				{
					Game: entity.Game{
						ID:   "game-1",
						Name: "Wingspan",
					},
					SectionsCount: 10,
				},
				{
					Game: entity.Game{
						ID:   "game-2",
						Name: "Azul",
					},
					SectionsCount: 5,
				},
			}, nil
		},
	}

	uc := usecase.NewListGamesUseCase(gameRepo)

	// Act
	games, err := uc.Execute(context.Background())

	// Assert
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(games) != 2 {
		t.Fatalf("Expected 2 games, got %d", len(games))
	}

	if games[0].Name != "Wingspan" || games[0].SectionsCount != 10 {
		t.Errorf("Expected first game to be 'Wingspan' with 10 sections, got '%s' with %d sections",
			games[0].Name, games[0].SectionsCount)
	}

	if games[1].Name != "Azul" || games[1].SectionsCount != 5 {
		t.Errorf("Expected second game to be 'Azul' with 5 sections, got '%s' with %d sections",
			games[1].Name, games[1].SectionsCount)
	}
}

// Test : Liste vide
func TestListGamesUseCase_Execute_EmptyList(t *testing.T) {
	// Arrange
	gameRepo := &mockGameRepo{
		listFunc: func(ctx context.Context) ([]*entity.GameWithStats, error) {
			return []*entity.GameWithStats{}, nil
		},
	}

	uc := usecase.NewListGamesUseCase(gameRepo)

	// Act
	games, err := uc.Execute(context.Background())

	// Assert
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(games) != 0 {
		t.Errorf("Expected empty list, got %d games", len(games))
	}
}

// Test : Erreur du repository
func TestListGamesUseCase_Execute_RepositoryError(t *testing.T) {
	// Arrange
	expectedErr := errors.New("database connection failed")
	gameRepo := &mockGameRepo{
		listFunc: func(ctx context.Context) ([]*entity.GameWithStats, error) {
			return nil, expectedErr
		},
	}

	uc := usecase.NewListGamesUseCase(gameRepo)

	// Act
	games, err := uc.Execute(context.Background())

	// Assert
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	if err != expectedErr {
		t.Errorf("Expected error '%v', got '%v'", expectedErr, err)
	}

	if games != nil {
		t.Errorf("Expected nil games on error, got %d games", len(games))
	}
}
