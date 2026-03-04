// application/usecase/import_game.go — Use case pour importer un jeu
package usecase

import (
	"context"
	"fmt"
	"log"
	"mime/multipart"
	"runtime"
	"time"

	"ask-rules-server/internal/domain/entity"
	"ask-rules-server/internal/domain/repository"
	"ask-rules-server/internal/infrastructure/storage"
)

// PipelineService interface pour le pipeline d'import.
type PipelineService interface {
	Process(ctx context.Context, opts *ImportOptions) error
}

// ImportOptions paramètres d'import.
type ImportOptions struct {
	GameName  string
	FilePaths []string
	OnEvent   func(event string, data map[string]interface{})
}

// ImportGameUseCase gère la logique métier pour importer un jeu.
type ImportGameUseCase struct {
	gameRepo    repository.GameRepository
	sectionRepo repository.SectionRepository
	pipeline    PipelineService
	uploadsDir  string
	logRepo     LogRepository
}

// NewImportGameUseCase crée un nouveau use case.
func NewImportGameUseCase(
	gameRepo repository.GameRepository,
	sectionRepo repository.SectionRepository,
	pipeline PipelineService,
	uploadsDir string,
	logRepo LogRepository,
) *ImportGameUseCase {
	return &ImportGameUseCase{
		gameRepo:    gameRepo,
		sectionRepo: sectionRepo,
		pipeline:    pipeline,
		uploadsDir:  uploadsDir,
		logRepo:     logRepo,
	}
}

// ImportRequest représente la requête d'import.
type ImportRequest struct {
	GameName string
	Files    []*multipart.FileHeader
	Mode     string // "replace" ou "merge"
	OnEvent  func(event string, data map[string]interface{})
}

// Execute importe un ou plusieurs fichiers pour un jeu.
func (uc *ImportGameUseCase) Execute(ctx context.Context, req *ImportRequest) error {
	emit := req.OnEvent
	if emit == nil {
		emit = func(string, map[string]interface{}) {}
	}

	// Validation
	if req.GameName == "" {
		return fmt.Errorf("game name is required")
	}
	if len(req.Files) == 0 {
		return fmt.Errorf("no files provided")
	}

	emit("start", map[string]interface{}{"game": req.GameName, "files": len(req.Files)})

	// Vérifier si le jeu existe déjà
	existingGame, _ := uc.gameRepo.FindByName(ctx, req.GameName)
	isNew := existingGame == nil
	slug := slugify(req.GameName)

	var game *entity.Game
	if isNew {
		// Créer un nouveau jeu
		game = &entity.Game{
			ID:       slug,
			Name:     req.GameName,
			AddedAt:  time.Now(),
			Gameplay: make(map[string]interface{}),
		}
	} else {
		game = existingGame
	}

	// Sauvegarder les fichiers uploadés
	var filePaths []string

	for _, fh := range req.Files {
		emit("uploading", map[string]interface{}{"file": fh.Filename})

		filePath, err := storage.SaveUploadedFile(fh, slug)
		if err != nil {
			log.Printf("[ERROR] ImportGame - Failed to save file '%s' for game '%s': %v", fh.Filename, req.GameName, err)
			emit("upload_error", map[string]interface{}{
				"file":  fh.Filename,
				"error": err.Error(),
			})
			continue
		}

		filePaths = append(filePaths, filePath)
	}

	if len(filePaths) == 0 {
		return fmt.Errorf("no files saved successfully")
	}

	// Sauvegarder le jeu
	if game.Stats == nil {
		game.Stats = make(map[string]interface{})
	}
	// En mode merge, conserver les fichiers existants ; en mode replace, remplacer
	if !isNew && req.Mode == "merge" {
		existing := toStringSlice(game.Stats["files"])
		game.Stats["files"] = mergeUnique(existing, filePaths)
	} else {
		game.Stats["files"] = filePaths
	}
	if err := uc.gameRepo.Save(ctx, game); err != nil {
		log.Printf("[ERROR] ImportGame - Failed to save game '%s': %v", req.GameName, err)
		return fmt.Errorf("failed to save game: %w", err)
	}

	// Si mode "replace", supprimer les sections existantes
	if !isNew && req.Mode == "replace" {
		emit("replacing", map[string]interface{}{"game": req.GameName})
		if err := uc.sectionRepo.DeleteByGameID(ctx, game.ID); err != nil {
			log.Printf("[ERROR] ImportGame - Failed to delete existing sections for game '%s': %v", req.GameName, err)
			return fmt.Errorf("failed to delete existing sections: %w", err)
		}
	}

	// Lancer le pipeline d'extraction et d'indexation
	start := time.Now()
	err := uc.pipeline.Process(ctx, &ImportOptions{
		GameName:  req.GameName,
		FilePaths: filePaths,
		OnEvent:   emit,
	})

	if err != nil {
		log.Printf("[ERROR] ImportGame - Pipeline failed for game '%s': %v", req.GameName, err)
		return fmt.Errorf("pipeline failed: %w", err)
	}

	// Forcer garbage collection après import (libérer RAM)
	runtime.GC()

	duration := time.Since(start)

	// Logger l'import en base de données
	if uc.logRepo != nil {
		_ = uc.logRepo.Save(ctx, &LogEntry{
			EventType: "game_import",
			Message:   fmt.Sprintf("Import du jeu '%s' (%d fichier(s))", req.GameName, len(filePaths)),
			Metadata: map[string]interface{}{
				"game":     req.GameName,
				"files":    len(filePaths),
				"mode":     req.Mode,
				"duration": duration.Milliseconds(),
				"new_game": isNew,
			},
		})
	}

	emit("complete", map[string]interface{}{
		"game":     req.GameName,
		"files":    len(filePaths),
		"duration": duration.Milliseconds(),
	})

	return nil
}

// mergeUnique fusionne deux slices en éliminant les doublons.
func mergeUnique(existing, newPaths []string) []string {
	seen := make(map[string]struct{}, len(existing))
	result := make([]string, 0, len(existing)+len(newPaths))
	for _, p := range existing {
		if _, ok := seen[p]; !ok {
			seen[p] = struct{}{}
			result = append(result, p)
		}
	}
	for _, p := range newPaths {
		if _, ok := seen[p]; !ok {
			seen[p] = struct{}{}
			result = append(result, p)
		}
	}
	return result
}
