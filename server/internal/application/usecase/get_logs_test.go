package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"ask-rules-server/internal/application/usecase"
)

// Mock LogRepository
type mockLogRepo struct {
	getRecentFunc func(ctx context.Context, limit int) ([]*usecase.LogEntry, error)
}

func (m *mockLogRepo) GetRecent(ctx context.Context, limit int) ([]*usecase.LogEntry, error) {
	if m.getRecentFunc != nil {
		return m.getRecentFunc(ctx, limit)
	}
	return []*usecase.LogEntry{}, nil
}

// Test : Récupérer des logs avec succès
func TestGetLogsUseCase_Execute_Success(t *testing.T) {
	// Arrange
	now := time.Now()
	logRepo := &mockLogRepo{
		getRecentFunc: func(ctx context.Context, limit int) ([]*usecase.LogEntry, error) {
			return []*usecase.LogEntry{
				{
					ID:        1,
					EventType: "game_added",
					Message:   "Game Wingspan added",
					Metadata:  map[string]interface{}{"game": "Wingspan"},
					CreatedAt: now.Add(-1 * time.Hour),
				},
				{
					ID:        2,
					EventType: "question_asked",
					Message:   "Question asked about Azul",
					Metadata:  map[string]interface{}{"game": "Azul"},
					CreatedAt: now,
				},
			}, nil
		},
	}

	uc := usecase.NewGetLogsUseCase(logRepo)

	// Act
	logs, err := uc.Execute(context.Background(), 100)

	// Assert
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(logs) != 2 {
		t.Fatalf("Expected 2 logs, got %d", len(logs))
	}

	if logs[0].EventType != "game_added" {
		t.Errorf("Expected first log type 'game_added', got '%s'", logs[0].EventType)
	}

	if logs[1].EventType != "question_asked" {
		t.Errorf("Expected second log type 'question_asked', got '%s'", logs[1].EventType)
	}
}

// Test : Limite par défaut
func TestGetLogsUseCase_Execute_DefaultLimit(t *testing.T) {
	// Arrange
	var actualLimit int
	logRepo := &mockLogRepo{
		getRecentFunc: func(ctx context.Context, limit int) ([]*usecase.LogEntry, error) {
			actualLimit = limit
			return []*usecase.LogEntry{}, nil
		},
	}

	uc := usecase.NewGetLogsUseCase(logRepo)

	// Act - Demander une limite invalide
	uc.Execute(context.Background(), 0)

	// Assert
	if actualLimit != 100 {
		t.Errorf("Expected default limit 100, got %d", actualLimit)
	}

	// Act - Demander une limite trop grande
	uc.Execute(context.Background(), 1000)

	// Assert
	if actualLimit != 100 {
		t.Errorf("Expected max limit 100, got %d", actualLimit)
	}
}

// Test : Limite personnalisée valide
func TestGetLogsUseCase_Execute_CustomLimit(t *testing.T) {
	// Arrange
	var actualLimit int
	logRepo := &mockLogRepo{
		getRecentFunc: func(ctx context.Context, limit int) ([]*usecase.LogEntry, error) {
			actualLimit = limit
			return []*usecase.LogEntry{}, nil
		},
	}

	uc := usecase.NewGetLogsUseCase(logRepo)

	// Act
	uc.Execute(context.Background(), 50)

	// Assert
	if actualLimit != 50 {
		t.Errorf("Expected limit 50, got %d", actualLimit)
	}
}

// Test : Liste vide
func TestGetLogsUseCase_Execute_EmptyList(t *testing.T) {
	// Arrange
	logRepo := &mockLogRepo{
		getRecentFunc: func(ctx context.Context, limit int) ([]*usecase.LogEntry, error) {
			return []*usecase.LogEntry{}, nil
		},
	}

	uc := usecase.NewGetLogsUseCase(logRepo)

	// Act
	logs, err := uc.Execute(context.Background(), 100)

	// Assert
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(logs) != 0 {
		t.Errorf("Expected empty list, got %d logs", len(logs))
	}
}

// Test : Erreur du repository
func TestGetLogsUseCase_Execute_RepositoryError(t *testing.T) {
	// Arrange
	expectedErr := errors.New("database error")
	logRepo := &mockLogRepo{
		getRecentFunc: func(ctx context.Context, limit int) ([]*usecase.LogEntry, error) {
			return nil, expectedErr
		},
	}

	uc := usecase.NewGetLogsUseCase(logRepo)

	// Act
	logs, err := uc.Execute(context.Background(), 100)

	// Assert
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	if logs != nil {
		t.Errorf("Expected nil logs on error, got %d logs", len(logs))
	}
}
