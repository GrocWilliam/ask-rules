// application/usecase/manage_files.go — Use cases pour gérer les fichiers
package usecase

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FileService interface pour gérer les fichiers.
type FileService interface {
	List(ctx context.Context, baseDir string) ([]*FileInfo, error)
	Delete(ctx context.Context, baseDir, slug, filename string) error
	Serve(ctx context.Context, baseDir, slug, filename string) (string, error)
}

// FileInfo représente les métadonnées d'un fichier.
type FileInfo struct {
	Game         string    `json:"game"`
	Name         string    `json:"name"`
	Size         int64     `json:"size"`
	Modified     time.Time `json:"modified"`
	RelativePath string    `json:"relative_path"`
	Path         string    `json:"path"`
}

// ListFilesUseCase gère la logique métier pour lister les fichiers.
type ListFilesUseCase struct {
	baseDir string
}

// NewListFilesUseCase crée un nouveau use case.
func NewListFilesUseCase(baseDir string) *ListFilesUseCase {
	return &ListFilesUseCase{baseDir: baseDir}
}

// Execute liste tous les fichiers dans le répertoire uploads.
func (uc *ListFilesUseCase) Execute(ctx context.Context) ([]*FileInfo, error) {
	var files []*FileInfo

	entries, err := os.ReadDir(uc.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*FileInfo{}, nil
		}
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		slug := entry.Name()
		gameDir := filepath.Join(uc.baseDir, slug)

		gameFiles, err := os.ReadDir(gameDir)
		if err != nil {
			continue
		}

		for _, file := range gameFiles {
			if file.IsDir() {
				continue
			}

			info, err := file.Info()
			if err != nil {
				continue
			}

			files = append(files, &FileInfo{
				Game:         slug,
				Name:         file.Name(),
				Size:         info.Size(),
				Modified:     info.ModTime(),
				RelativePath: filepath.Join(slug, file.Name()),
				Path:         filepath.Join(gameDir, file.Name()),
			})
		}
	}

	return files, nil
}

// DeleteFileUseCase gère la logique métier pour supprimer un fichier.
type DeleteFileUseCase struct {
	baseDir string
}

// NewDeleteFileUseCase crée un nouveau use case.
func NewDeleteFileUseCase(baseDir string) *DeleteFileUseCase {
	return &DeleteFileUseCase{baseDir: baseDir}
}

// Execute supprime un fichier du système de fichiers.
func (uc *DeleteFileUseCase) Execute(ctx context.Context, slug, filename string) error {
	// Validation : interdire les path traversals
	if strings.Contains(slug, "..") || strings.Contains(filename, "..") {
		return fmt.Errorf("invalid path: contains '..'")
	}

	if slug == "" || filename == "" {
		return fmt.Errorf("slug and filename are required")
	}

	absPath := filepath.Join(uc.baseDir, slug, filename)

	// Vérifier que le fichier existe
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return fmt.Errorf("file not found: %w", err)
	}

	// Supprimer le fichier
	if err := os.Remove(absPath); err != nil {
		return fmt.Errorf("failed to delete file: %w", err)
	}

	// Essayer de supprimer le répertoire parent s'il est vide (silencieux)
	parentDir := filepath.Join(uc.baseDir, slug)
	os.Remove(parentDir)

	return nil
}

// ServeFileUseCase gère la logique métier pour servir un fichier.
type ServeFileUseCase struct {
	baseDir string
}

// NewServeFileUseCase crée un nouveau use case.
func NewServeFileUseCase(baseDir string) *ServeFileUseCase {
	return &ServeFileUseCase{baseDir: baseDir}
}

// Execute retourne le chemin absolu d'un fichier à servir.
func (uc *ServeFileUseCase) Execute(ctx context.Context, slug, filename string) (string, error) {
	// Validation : interdire les path traversals
	if strings.Contains(slug, "..") || strings.Contains(filename, "..") {
		return "", fmt.Errorf("invalid path: contains '..'")
	}

	if slug == "" || filename == "" {
		return "", fmt.Errorf("slug and filename are required")
	}

	absPath := filepath.Join(uc.baseDir, slug, filename)

	// Vérifier que le fichier existe
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return "", fmt.Errorf("file not found: %w", err)
	}

	return absPath, nil
}
