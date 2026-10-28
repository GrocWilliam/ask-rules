// index/reindex.go — Remplacement des sections d'un jeu par une nouvelle indexation
package index

import (
	"context"
	"fmt"
	"log"
	"slices"

	"ask-rules-server/internal/store"
)

// reindex lance index (le pipeline, qui insère les nouvelles sections) puis,
// si replace, supprime les sections d'avant. Les anciennes sections restent
// en place tant que l'indexation n'a pas réussi : un échec (PDF illisible,
// pdftotext absent…) laisse le jeu tel qu'il était au lieu de le vider, et
// les sections insérées par l'indexation échouée sont retirées.
// Pendant l'indexation, les questions voient les anciennes sections et les
// nouvelles déjà insérées.
func reindex(
	ctx context.Context,
	gameRepo store.GameRepository,
	sectionRepo store.SectionRepository,
	game *store.Game,
	replace bool,
	emit func(string, map[string]interface{}),
	index func() error,
) error {
	oldIDs, err := sectionRepo.ListIDsByGameID(ctx, game.ID)
	if err != nil {
		return fmt.Errorf("failed to list existing sections: %w", err)
	}

	if err := index(); err != nil {
		if cleanupErr := deleteNewSections(ctx, sectionRepo, game.ID, oldIDs); cleanupErr != nil {
			log.Printf("[ERROR] Reindex - Failed to remove partial sections for game '%s': %v", game.Name, cleanupErr)
		}
		return err
	}

	if replace && len(oldIDs) > 0 {
		if err := sectionRepo.DeleteByIDs(ctx, oldIDs); err != nil {
			return fmt.Errorf("failed to delete previous sections: %w", err)
		}
		emit("replacing", map[string]interface{}{"game": game.Name})
	}

	// Nouvelle date_update : les réponses en cache pour ce jeu sont périmées
	if err := gameRepo.Touch(ctx, game.ID); err != nil {
		log.Printf("[WARN] Reindex - Failed to touch game '%s': %v", game.Name, err)
	}
	return nil
}

// deleteNewSections supprime les sections du jeu absentes de oldIDs.
func deleteNewSections(ctx context.Context, sectionRepo store.SectionRepository, gameID string, oldIDs []string) error {
	ids, err := sectionRepo.ListIDsByGameID(ctx, gameID)
	if err != nil {
		return err
	}
	ids = slices.DeleteFunc(ids, func(id string) bool { return slices.Contains(oldIDs, id) })
	return sectionRepo.DeleteByIDs(ctx, ids)
}
