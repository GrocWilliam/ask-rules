// index/delete.go — Use case pour supprimer un jeu
package index

import (
	"context"
	"errors"
	"fmt"
	"log"

	"ask-rules-server/internal/files"
	"ask-rules-server/internal/store"
)

// Delete supprime un jeu et toutes ses sections associées.
func (s *Service) Delete(ctx context.Context, gameID string) error {
	// 1. Vérifier que le jeu existe
	game, err := s.gameRepo.FindByID(ctx, gameID)
	if err != nil {
		if errors.Is(err, store.ErrGameNotFound) {
			log.Printf("[ERROR] DeleteGame - Game '%s' not found", gameID)
			return fmt.Errorf("game '%s' not found: %w", gameID, store.ErrGameNotFound)
		}
		log.Printf("[ERROR] DeleteGame - Failed to find game '%s': %v", gameID, err)
		return fmt.Errorf("failed to find game: %w", err)
	}

	// 2. Supprimer les sections
	if err := s.sectionRepo.DeleteByGameID(ctx, gameID); err != nil {
		log.Printf("[ERROR] DeleteGame - Failed to delete sections for game '%s': %v", game.Name, err)
		return fmt.Errorf("failed to delete sections: %w", err)
	}

	// 3. Supprimer le jeu
	if err := s.gameRepo.Delete(ctx, gameID); err != nil {
		return fmt.Errorf("failed to delete game: %w", err)
	}

	// 4. Supprimer les fichiers : sinon ils restent téléchargeables via /files/
	files.DeleteGameFiles(game.Files())

	// Cache : la clé dépend de date_update, un jeu réimporté sous le même ID
	// ne retrouve donc pas les anciennes réponses

	return nil
}
