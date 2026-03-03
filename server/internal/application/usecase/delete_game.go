// application/usecase/delete_game.go — Use case pour supprimer un jeu
package usecase

import (
	"context"
	"fmt"
	"log"

	"ask-rules-server/internal/domain/entity"
	"ask-rules-server/internal/domain/repository"
	"ask-rules-server/internal/domain/service"
)

// DeleteGameUseCase gère la logique métier pour supprimer un jeu.
type DeleteGameUseCase struct {
	gameRepo    repository.GameRepository
	sectionRepo repository.SectionRepository
	cache       service.CacheService
}

// NewDeleteGameUseCase crée un nouveau use case.
func NewDeleteGameUseCase(
	gameRepo repository.GameRepository,
	sectionRepo repository.SectionRepository,
	cache service.CacheService,
) *DeleteGameUseCase {
	return &DeleteGameUseCase{
		gameRepo:    gameRepo,
		sectionRepo: sectionRepo,
		cache:       cache,
	}
}

// Execute supprime un jeu et toutes ses sections associées.
func (uc *DeleteGameUseCase) Execute(ctx context.Context, gameID string) error {
	// 1. Vérifier que le jeu existe
	game, err := uc.gameRepo.FindByID(ctx, gameID)
	if err != nil {
		if err == entity.ErrGameNotFound {
			log.Printf("[ERROR] DeleteGame - Game '%s' not found", gameID)
			return fmt.Errorf("game '%s' not found: %w", gameID, entity.ErrGameNotFound)
		}
		log.Printf("[ERROR] DeleteGame - Failed to find game '%s': %v", gameID, err)
		return fmt.Errorf("failed to find game: %w", err)
	}

	// 2. Supprimer les sections
	if err := uc.sectionRepo.DeleteByGameID(ctx, gameID); err != nil {
		log.Printf("[ERROR] DeleteGame - Failed to delete sections for game '%s': %v", game.Name, err)
		return fmt.Errorf("failed to delete sections: %w", err)
	}

	// 3. Supprimer le jeu
	if err := uc.gameRepo.Delete(ctx, gameID); err != nil {
		return fmt.Errorf("failed to delete game: %w", err)
	}

	// 4. Invalider le cache (best effort, ne pas échouer si erreur)
	// On ne peut pas invalider toutes les clés facilement, Redis SCAN serait nécessaire
	// Pour l'instant, on laisse le cache expirer naturellement (24h TTL)
	_ = game // Pour éviter unused warning

	return nil
}
