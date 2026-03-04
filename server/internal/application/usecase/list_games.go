// application/usecase/list_games.go — Use case pour lister les jeux
package usecase

import (
	"context"

	"ask-rules-server/internal/domain/repository"
)

// ListGamesUseCase gère la logique métier pour lister tous les jeux.
type ListGamesUseCase struct {
	gameRepo repository.GameRepository
}

// NewListGamesUseCase crée un nouveau use case.
func NewListGamesUseCase(gameRepo repository.GameRepository) *ListGamesUseCase {
	return &ListGamesUseCase{gameRepo: gameRepo}
}

// GameDTO représente un jeu dans la réponse.
type GameDTO struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	SectionCount int    `json:"section_count"`
}

// Execute liste tous les jeux avec leur nombre de sections.
func (uc *ListGamesUseCase) Execute(ctx context.Context) ([]*GameDTO, error) {
	games, err := uc.gameRepo.List(ctx)
	if err != nil {
		return nil, err
	}

	// Mapper entity → DTO
	result := make([]*GameDTO, len(games))
	for i, game := range games {
		result[i] = &GameDTO{
			ID:           game.ID,
			Name:         game.Name,
			SectionCount: game.SectionsCount,
		}
	}

	return result, nil
}
