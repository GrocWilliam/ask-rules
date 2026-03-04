// application/usecase/get_game.go — Use case pour obtenir les détails d'un jeu
package usecase

import (
	"context"
	"fmt"
	"time"

	"ask-rules-server/internal/domain/entity"
	"ask-rules-server/internal/domain/repository"
)

// GetGameUseCase gère la logique métier pour obtenir les détails d'un jeu.
type GetGameUseCase struct {
	gameRepo repository.GameRepository
}

// NewGetGameUseCase crée un nouveau use case.
func NewGetGameUseCase(gameRepo repository.GameRepository) *GetGameUseCase {
	return &GetGameUseCase{gameRepo: gameRepo}
}

// GameDetailDTO représente les détails complets d'un jeu.
type GameDetailDTO struct {
	ID       string                 `json:"id"`
	Name     string                 `json:"name"`
	AddedAt  time.Time              `json:"added_at"`
	Gameplay map[string]interface{} `json:"gameplay,omitempty"`
}

// Execute récupère les détails d'un jeu par son ID.
func (uc *GetGameUseCase) Execute(ctx context.Context, gameID string) (*GameDetailDTO, error) {
	if gameID == "" {
		return nil, fmt.Errorf("game ID is required")
	}

	game, err := uc.gameRepo.FindByID(ctx, gameID)
	if err != nil {
		if err == entity.ErrGameNotFound {
			return nil, fmt.Errorf("game '%s' not found: %w", gameID, entity.ErrGameNotFound)
		}
		return nil, fmt.Errorf("failed to find game: %w", err)
	}

	// Mapper entity → DTO
	return &GameDetailDTO{
		ID:       game.ID,
		Name:     game.Name,
		AddedAt:  game.AddedAt,
		Gameplay: game.Gameplay}, nil
}
