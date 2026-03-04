// application/usecase/reprocess_game.go — Use case pour reprocesser un jeu existant
package usecase

import (
	"context"
	"fmt"
	"log"
	"runtime"
	"time"

	"ask-rules-server/internal/domain/entity"
	"ask-rules-server/internal/domain/repository"
)

// ReprocessGameUseCase relance le pipeline d'indexation sur les fichiers existants d'un jeu.
type ReprocessGameUseCase struct {
	gameRepo    repository.GameRepository
	sectionRepo repository.SectionRepository
	pipeline    PipelineService
	uploadsDir  string
	logRepo     LogRepository
}

// NewReprocessGameUseCase crée un nouveau use case.
func NewReprocessGameUseCase(
	gameRepo repository.GameRepository,
	sectionRepo repository.SectionRepository,
	pipeline PipelineService,
	uploadsDir string,
	logRepo LogRepository,
) *ReprocessGameUseCase {
	return &ReprocessGameUseCase{
		gameRepo:    gameRepo,
		sectionRepo: sectionRepo,
		pipeline:    pipeline,
		uploadsDir:  uploadsDir,
		logRepo:     logRepo,
	}
}

// ReprocessRequest est la requête de reprocess.
type ReprocessRequest struct {
	GameID  string
	OnEvent func(event string, data map[string]interface{})
}

// Execute relance le pipeline sur les fichiers déjà importés du jeu.
func (uc *ReprocessGameUseCase) Execute(ctx context.Context, req *ReprocessRequest) error {
	emit := req.OnEvent
	if emit == nil {
		emit = func(string, map[string]interface{}) {}
	}

	// 1. Récupérer le jeu
	game, err := uc.gameRepo.FindByID(ctx, req.GameID)
	if err != nil {
		if err == entity.ErrGameNotFound {
			return fmt.Errorf("game '%s' not found: %w", req.GameID, entity.ErrGameNotFound)
		}
		return fmt.Errorf("failed to find game: %w", err)
	}

	// 2. Récupérer les chemins de fichiers depuis les stats
	filePaths := toStringSlice(game.Stats["files"])
	if len(filePaths) == 0 {
		return fmt.Errorf("no files found for game '%s'", game.Name)
	}

	emit("start", map[string]interface{}{
		"game":  game.Name,
		"files": len(filePaths),
	})

	// 3. Supprimer les sections existantes
	emit("replacing", map[string]interface{}{"game": game.Name})
	if err := uc.sectionRepo.DeleteByGameID(ctx, game.ID); err != nil {
		log.Printf("[ERROR] ReprocessGame - Failed to delete sections for game '%s': %v", game.Name, err)
		return fmt.Errorf("failed to delete existing sections: %w", err)
	}

	// 4. Relancer le pipeline
	start := time.Now()
	if err := uc.pipeline.Process(ctx, &ImportOptions{
		GameName:  game.Name,
		FilePaths: filePaths,
		OnEvent:   mapPipelineEvents(emit),
	}); err != nil {
		log.Printf("[ERROR] ReprocessGame - Pipeline failed for game '%s': %v", game.Name, err)
		return fmt.Errorf("pipeline failed: %w", err)
	}

	runtime.GC()

	duration := time.Since(start)

	// Logger le reprocess en base de données
	if uc.logRepo != nil {
		_ = uc.logRepo.Save(ctx, &LogEntry{
			EventType: "game_reprocess",
			Message:   fmt.Sprintf("Reprocess du jeu '%s' (%d fichier(s))", game.Name, len(filePaths)),
			Metadata: map[string]interface{}{
				"game":     game.Name,
				"files":    len(filePaths),
				"duration": duration.Milliseconds(),
			},
		})
	}

	emit("complete", map[string]interface{}{
		"game":     game.Name,
		"files":    len(filePaths),
		"duration": duration.Milliseconds(),
	})

	return nil
}

// ExecuteAll relance le pipeline sur tous les jeux existants, un par un.
func (uc *ReprocessGameUseCase) ExecuteAll(ctx context.Context, emit func(string, map[string]interface{})) {
	if emit == nil {
		emit = func(string, map[string]interface{}) {}
	}

	// 1. Lister tous les jeux
	games, err := uc.gameRepo.List(ctx)
	if err != nil {
		emit("error", map[string]interface{}{"error": err.Error()})
		return
	}

	total := len(games)
	emit("start", map[string]interface{}{"total": total})

	globalStart := time.Now()
	successes := 0

	for i, gws := range games {
		select {
		case <-ctx.Done():
			emit("error", map[string]interface{}{"error": "cancelled"})
			return
		default:
		}

		game := &gws.Game
		emit("game_start", map[string]interface{}{
			"index": i + 1,
			"total": total,
			"game":  game.Name,
		})

		filePaths := toStringSlice(game.Stats["files"])
		if len(filePaths) == 0 {
			emit("game_error", map[string]interface{}{
				"game":  game.Name,
				"error": "no files found",
			})
			continue
		}

		// Supprimer les sections existantes
		if err := uc.sectionRepo.DeleteByGameID(ctx, game.ID); err != nil {
			log.Printf("[ERROR] ReprocessAll - Failed to delete sections for '%s': %v", game.Name, err)
			emit("game_error", map[string]interface{}{
				"game":  game.Name,
				"error": err.Error(),
			})
			continue
		}

		// Relancer le pipeline en relayant les événements
		pipelineErr := uc.pipeline.Process(ctx, &ImportOptions{
			GameName:  game.Name,
			FilePaths: filePaths,
			OnEvent:   mapPipelineEvents(emit),
		})

		if pipelineErr != nil {
			log.Printf("[ERROR] ReprocessAll - Pipeline failed for '%s': %v", game.Name, pipelineErr)
			emit("game_error", map[string]interface{}{
				"game":  game.Name,
				"error": pipelineErr.Error(),
			})
			continue
		}

		successes++
		emit("game_done", map[string]interface{}{"game": game.Name})
		runtime.GC()
	}

	duration := time.Since(globalStart)

	// Logger le reprocess-all en base de données
	if uc.logRepo != nil {
		_ = uc.logRepo.Save(context.Background(), &LogEntry{
			EventType: "game_reprocess_all",
			Message:   fmt.Sprintf("Reprocess de tous les jeux: %d/%d réussis", successes, total),
			Metadata: map[string]interface{}{
				"success":  successes,
				"total":    total,
				"duration": duration.Milliseconds(),
			},
		})
	}

	emit("complete", map[string]interface{}{
		"success":  successes,
		"total":    total,
		"duration": duration.Milliseconds(),
	})
}

// mapPipelineEvents convertit les événements bruts du pipeline vers les événements SSE attendus par le frontend.
//
// Pipeline émet : extracting, chunking, embedding{file,chunks}, progress{file,done,total},
//
//	done, file_error, section_error, gameplay, gameplay_error
//
// Frontend attend : step{message}, embedding_start{total}, embedding_progress{current,total}, complete, error
func mapPipelineEvents(emit func(string, map[string]interface{})) func(string, map[string]interface{}) {
	return func(event string, data map[string]interface{}) {
		switch event {
		case "extracting":
			file, _ := data["file"].(string)
			emit("step", map[string]interface{}{"message": "Extraction : " + file})
		case "chunking":
			file, _ := data["file"].(string)
			emit("step", map[string]interface{}{"message": "Découpage : " + file})
		case "embedding":
			// chunks peut être int ou float64 selon la source
			var total int
			switch v := data["chunks"].(type) {
			case int:
				total = v
			case float64:
				total = int(v)
			}
			emit("embedding_start", map[string]interface{}{"total": total})
		case "progress":
			var current, total int
			switch v := data["done"].(type) {
			case int:
				current = v
			case float64:
				current = int(v)
			}
			switch v := data["total"].(type) {
			case int:
				total = v
			case float64:
				total = int(v)
			}
			emit("embedding_progress", map[string]interface{}{"current": current, "total": total})
		case "done":
			// événement fin de pipeline — ignoré, le use case émet "complete" lui-même
		case "file_error", "section_error":
			file, _ := data["file"].(string)
			errMsg, _ := data["error"].(string)
			msg := event
			if file != "" {
				msg += " " + file
			}
			if errMsg != "" {
				msg += ": " + errMsg
			}
			emit("step", map[string]interface{}{"message": "⚠️ " + msg})
		case "gameplay":
			emit("step", map[string]interface{}{"message": "Extraction gameplay..."})
		case "gameplay_error":
			errMsg, _ := data["error"].(string)
			emit("step", map[string]interface{}{"message": "⚠️ Gameplay : " + errMsg})
		// événements déjà bien formés (import direct)
		case "step":
			emit("step", data)
		}
	}
}
