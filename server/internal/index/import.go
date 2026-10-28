// index/import.go — Use case pour importer un jeu
package index

import (
	"context"
	"fmt"
	"log"
	"mime/multipart"
	"runtime"
	"slices"
	"time"

	"ask-rules-server/internal/files"
	"ask-rules-server/internal/store"
)

// ImportOptions paramètres d'import.
type ImportOptions struct {
	GameName  string
	FilePaths []string
	OnEvent   func(event string, data map[string]interface{})
}

// ImportRequest représente la requête d'import.
type ImportRequest struct {
	GameName       string
	Files          []*multipart.FileHeader
	PreCopiedPaths []string // Chemins de fichiers déjà copiés dans uploads/ (pour les URLs)
	Mode           string   // "replace" ou "merge"
	OnEvent        func(event string, data map[string]interface{})
}

// Import importe un ou plusieurs fichiers pour un jeu.
func (s *Service) Import(ctx context.Context, req *ImportRequest) error {
	emit := req.OnEvent
	if emit == nil {
		emit = func(string, map[string]interface{}) {}
	}

	// Validation
	if req.GameName == "" {
		return fmt.Errorf("game name is required")
	}
	if len(req.Files) == 0 && len(req.PreCopiedPaths) == 0 {
		return fmt.Errorf("no files provided")
	}

	emit("start", map[string]interface{}{"game": req.GameName, "files": len(req.Files)})

	// Vérifier si le jeu existe déjà
	existingGame, _ := s.gameRepo.FindByName(ctx, req.GameName)
	isNew := existingGame == nil
	slug := Slugify(req.GameName)

	var game *store.Game
	if isNew {
		// Créer un nouveau jeu
		game = &store.Game{
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

	// Traiter les fichiers uploadés via multipart
	for _, fh := range req.Files {
		emit("uploading", map[string]interface{}{"file": fh.Filename})

		filePath, err := files.SaveUploadedFile(fh, slug)
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

	// Ajouter les fichiers déjà copiés (URLs)
	filePaths = append(filePaths, req.PreCopiedPaths...)

	if len(filePaths) == 0 {
		return fmt.Errorf("no files saved successfully")
	}

	// Sauvegarder le jeu
	if game.Stats == nil {
		game.Stats = make(map[string]interface{})
	}
	// En mode merge, conserver les fichiers existants ; en mode replace, remplacer
	previousFiles := game.Files()
	replace := isNew || req.Mode != "merge"
	var replaced []string
	if replace {
		for _, f := range previousFiles {
			if !slices.Contains(filePaths, f) {
				replaced = append(replaced, f)
			}
		}
		game.Stats["files"] = filePaths
	} else {
		game.Stats["files"] = mergeUnique(previousFiles, filePaths)
	}
	// Le pipeline retrouve le jeu en base : il doit être enregistré avant
	if err := s.gameRepo.Save(ctx, game); err != nil {
		log.Printf("[ERROR] ImportGame - Failed to save game '%s': %v", req.GameName, err)
		return fmt.Errorf("failed to save game: %w", err)
	}

	// Lancer le pipeline d'extraction et d'indexation ; les anciennes sections
	// ne sont remplacées qu'une fois l'indexation réussie
	start := time.Now()
	err := reindex(ctx, s.gameRepo, s.sectionRepo, game, replace, emit, func() error {
		return s.pipeline.Process(ctx, &ImportOptions{
			GameName:  req.GameName,
			FilePaths: filePaths,
			OnEvent:   emit,
		})
	})
	if err != nil {
		log.Printf("[ERROR] ImportGame - Pipeline failed for game '%s': %v", req.GameName, err)
		s.rollback(ctx, game, isNew, previousFiles, filePaths)
		return fmt.Errorf("pipeline failed: %w", err)
	}

	// Anciens fichiers remplacés : sinon ils restent téléchargeables via /files/
	files.DeleteGameFiles(replaced)

	// Forcer garbage collection après import (libérer RAM)
	runtime.GC()

	duration := time.Since(start)

	// Logger l'import en base de données
	if s.logRepo != nil {
		_ = s.logRepo.Save(ctx, &store.LogEntry{
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

// rollback remet le jeu dans son état d'avant un import échoué : un nouveau
// jeu est supprimé, un jeu existant retrouve sa liste de fichiers. Les
// fichiers ajoutés par l'import sont supprimés ; un fichier de même nom qu'un
// ancien l'a écrasé à l'upload et reste en place.
func (s *Service) rollback(ctx context.Context, game *store.Game, isNew bool, previousFiles, filePaths []string) {
	if isNew {
		if err := s.gameRepo.Delete(ctx, game.ID); err != nil {
			log.Printf("[ERROR] ImportGame - Failed to remove new game '%s' after failure: %v", game.Name, err)
		}
	} else {
		if len(previousFiles) > 0 {
			game.Stats["files"] = previousFiles
		} else {
			delete(game.Stats, "files")
		}
		if err := s.gameRepo.Save(ctx, game); err != nil {
			log.Printf("[ERROR] ImportGame - Failed to restore files of game '%s': %v", game.Name, err)
		}
	}

	var added []string
	for _, f := range filePaths {
		if !slices.Contains(previousFiles, f) {
			added = append(added, f)
		}
	}
	files.DeleteGameFiles(added)
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
