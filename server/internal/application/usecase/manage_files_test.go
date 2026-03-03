// application/usecase/manage_files_test.go — Tests unitaires des use cases ManageFiles
package usecase_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"ask-rules-server/internal/application/usecase"
)

// Helper pour créer un répertoire de test
func createTestDir(t *testing.T) (string, func()) {
	tmpDir, err := os.MkdirTemp("", "test-uploads-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	cleanup := func() {
		os.RemoveAll(tmpDir)
	}

	return tmpDir, cleanup
}

// Helper pour créer un fichier de test
func createTestFile(t *testing.T, baseDir, slug, filename, content string) {
	dir := filepath.Join(baseDir, slug)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}
}

// Tests pour ListFilesUseCase

func TestListFilesUseCase_Execute_Success(t *testing.T) {
	// Arrange
	baseDir, cleanup := createTestDir(t)
	defer cleanup()

	createTestFile(t, baseDir, "wingspan", "rules.pdf", "wingspan rules")
	createTestFile(t, baseDir, "wingspan", "quick-ref.pdf", "quick reference")
	createTestFile(t, baseDir, "azul", "manual.pdf", "azul manual")

	uc := usecase.NewListFilesUseCase(baseDir)

	// Act
	files, err := uc.Execute(context.Background())

	// Assert
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(files) != 3 {
		t.Fatalf("Expected 3 files, got %d", len(files))
	}

	// Vérifier qu'on a les bons jeux
	gameNames := make(map[string]int)
	for _, f := range files {
		gameNames[f.Game]++
	}

	if gameNames["wingspan"] != 2 {
		t.Errorf("Expected 2 files for 'wingspan', got %d", gameNames["wingspan"])
	}

	if gameNames["azul"] != 1 {
		t.Errorf("Expected 1 file for 'azul', got %d", gameNames["azul"])
	}
}

func TestListFilesUseCase_Execute_EmptyDirectory(t *testing.T) {
	// Arrange
	baseDir, cleanup := createTestDir(t)
	defer cleanup()

	uc := usecase.NewListFilesUseCase(baseDir)

	// Act
	files, err := uc.Execute(context.Background())

	// Assert
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(files) != 0 {
		t.Errorf("Expected empty list, got %d files", len(files))
	}
}

func TestListFilesUseCase_Execute_NonExistentDirectory(t *testing.T) {
	// Arrange
	uc := usecase.NewListFilesUseCase("/nonexistent/path")

	// Act
	files, err := uc.Execute(context.Background())

	// Assert
	if err != nil {
		t.Fatalf("Expected no error for nonexistent dir, got: %v", err)
	}

	if len(files) != 0 {
		t.Errorf("Expected empty list, got %d files", len(files))
	}
}

// Tests pour DeleteFileUseCase

func TestDeleteFileUseCase_Execute_Success(t *testing.T) {
	// Arrange
	baseDir, cleanup := createTestDir(t)
	defer cleanup()

	createTestFile(t, baseDir, "wingspan", "rules.pdf", "test content")

	uc := usecase.NewDeleteFileUseCase(baseDir)

	// Vérifier que le fichier existe avant
	filePath := filepath.Join(baseDir, "wingspan", "rules.pdf")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Fatal("File should exist before deletion")
	}

	// Act
	err := uc.Execute(context.Background(), "wingspan", "rules.pdf")

	// Assert
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Vérifier que le fichier n'existe plus
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Error("File should not exist after deletion")
	}
}

func TestDeleteFileUseCase_Execute_FileNotFound(t *testing.T) {
	// Arrange
	baseDir, cleanup := createTestDir(t)
	defer cleanup()

	uc := usecase.NewDeleteFileUseCase(baseDir)

	// Act
	err := uc.Execute(context.Background(), "wingspan", "nonexistent.pdf")

	// Assert
	if err == nil {
		t.Fatal("Expected error for nonexistent file, got nil")
	}
}

func TestDeleteFileUseCase_Execute_PathTraversal(t *testing.T) {
	// Arrange
	baseDir, cleanup := createTestDir(t)
	defer cleanup()

	uc := usecase.NewDeleteFileUseCase(baseDir)

	// Act - Tenter un path traversal dans le slug
	err := uc.Execute(context.Background(), "../etc", "passwd")

	// Assert
	if err == nil {
		t.Fatal("Expected error for path traversal, got nil")
	}

	// Act - Tenter un path traversal dans le filename
	err = uc.Execute(context.Background(), "wingspan", "../../../etc/passwd")

	// Assert
	if err == nil {
		t.Fatal("Expected error for path traversal, got nil")
	}
}

func TestDeleteFileUseCase_Execute_EmptyParameters(t *testing.T) {
	// Arrange
	baseDir, cleanup := createTestDir(t)
	defer cleanup()

	uc := usecase.NewDeleteFileUseCase(baseDir)

	// Act - Slug vide
	err := uc.Execute(context.Background(), "", "file.pdf")

	// Assert
	if err == nil {
		t.Fatal("Expected error for empty slug, got nil")
	}

	// Act - Filename vide
	err = uc.Execute(context.Background(), "game", "")

	// Assert
	if err == nil {
		t.Fatal("Expected error for empty filename, got nil")
	}
}

// Tests pour ServeFileUseCase

func TestServeFileUseCase_Execute_Success(t *testing.T) {
	// Arrange
	baseDir, cleanup := createTestDir(t)
	defer cleanup()

	createTestFile(t, baseDir, "wingspan", "rules.pdf", "test content")

	uc := usecase.NewServeFileUseCase(baseDir)

	// Act
	absPath, err := uc.Execute(context.Background(), "wingspan", "rules.pdf")

	// Assert
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	expectedPath := filepath.Join(baseDir, "wingspan", "rules.pdf")
	if absPath != expectedPath {
		t.Errorf("Expected path '%s', got '%s'", expectedPath, absPath)
	}

	// Vérifier que le fichier existe vraiment
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		t.Error("Returned path should point to existing file")
	}
}

func TestServeFileUseCase_Execute_FileNotFound(t *testing.T) {
	// Arrange
	baseDir, cleanup := createTestDir(t)
	defer cleanup()

	uc := usecase.NewServeFileUseCase(baseDir)

	// Act
	absPath, err := uc.Execute(context.Background(), "wingspan", "nonexistent.pdf")

	// Assert
	if err == nil {
		t.Fatal("Expected error for nonexistent file, got nil")
	}

	if absPath != "" {
		t.Errorf("Expected empty path on error, got '%s'", absPath)
	}
}

func TestServeFileUseCase_Execute_PathTraversal(t *testing.T) {
	// Arrange
	baseDir, cleanup := createTestDir(t)
	defer cleanup()

	uc := usecase.NewServeFileUseCase(baseDir)

	// Act - Tenter un path traversal dans le slug
	absPath, err := uc.Execute(context.Background(), "../etc", "passwd")

	// Assert
	if err == nil {
		t.Fatal("Expected error for path traversal, got nil")
	}

	if absPath != "" {
		t.Errorf("Expected empty path on error, got '%s'", absPath)
	}

	// Act - Tenter un path traversal dans le filename
	absPath, err = uc.Execute(context.Background(), "wingspan", "../../../etc/passwd")

	// Assert
	if err == nil {
		t.Fatal("Expected error for path traversal, got nil")
	}

	if absPath != "" {
		t.Errorf("Expected empty path on error, got '%s'", absPath)
	}
}

func TestServeFileUseCase_Execute_EmptyParameters(t *testing.T) {
	// Arrange
	baseDir, cleanup := createTestDir(t)
	defer cleanup()

	uc := usecase.NewServeFileUseCase(baseDir)

	// Act - Slug vide
	absPath, err := uc.Execute(context.Background(), "", "file.pdf")

	// Assert
	if err == nil {
		t.Fatal("Expected error for empty slug, got nil")
	}

	if absPath != "" {
		t.Errorf("Expected empty path on error, got '%s'", absPath)
	}

	// Act - Filename vide
	absPath, err = uc.Execute(context.Background(), "game", "")

	// Assert
	if err == nil {
		t.Fatal("Expected error for empty filename, got nil")
	}

	if absPath != "" {
		t.Errorf("Expected empty path on error, got '%s'", absPath)
	}
}
