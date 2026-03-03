// application/usecase/import_game.go — Use case pour importer un jeu
package usecase

import (
	"context"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"ask-rules-server/internal/domain/entity"
	"ask-rules-server/internal/domain/repository"
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
}

// NewImportGameUseCase crée un nouveau use case.
func NewImportGameUseCase(
	gameRepo repository.GameRepository,
	sectionRepo repository.SectionRepository,
	pipeline PipelineService,
	uploadsDir string,
) *ImportGameUseCase {
	return &ImportGameUseCase{
		gameRepo:    gameRepo,
		sectionRepo: sectionRepo,
		pipeline:    pipeline,
		uploadsDir:  uploadsDir,
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

	var game *entity.Game
	if isNew {
		// Créer un nouveau jeu
		game = &entity.Game{
			ID:       generateGameID(req.GameName),
			Name:     req.GameName,
			AddedAt:  time.Now(),
			Gameplay: make(map[string]interface{}),
		}
	} else {
		game = existingGame
	}

	// Sauvegarder les fichiers uploadés
	slug := slugify(req.GameName)
	var filePaths []string

	for _, fh := range req.Files {
		emit("uploading", map[string]interface{}{"file": fh.Filename})

		filePath, err := uc.saveFile(fh, slug)
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

	emit("complete", map[string]interface{}{
		"game":     req.GameName,
		"files":    len(filePaths),
		"duration": time.Since(start).Milliseconds(),
	})

	return nil
}

// saveFile sauvegarde un fichier uploadé sur le disque.
func (uc *ImportGameUseCase) saveFile(fh *multipart.FileHeader, slug string) (string, error) {
	// Ouvrir le fichier uploadé
	src, err := fh.Open()
	if err != nil {
		return "", fmt.Errorf("failed to open uploaded file: %w", err)
	}
	defer src.Close()

	// Créer le répertoire de destination
	dstDir := filepath.Join(uc.uploadsDir, slug)
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	// Générer un nom de fichier sécurisé
	filename := sanitizeFilename(fh.Filename)
	dstPath := filepath.Join(dstDir, filename)

	// Créer le fichier de destination
	dst, err := os.Create(dstPath)
	if err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}
	defer dst.Close()

	// Copier le contenu
	if _, err := io.Copy(dst, src); err != nil {
		return "", fmt.Errorf("failed to copy file: %w", err)
	}

	// Retourner le chemin relatif
	return filepath.Join(slug, filename), nil
}

// sanitizeFilename nettoie un nom de fichier.
func sanitizeFilename(name string) string {
	// Enlever les path traversals
	name = filepath.Base(name)
	// Remplacer les caractères dangereux
	name = strings.ReplaceAll(name, "..", "")
	return name
}
