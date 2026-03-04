// application/usecase/get_logs.go — Use case pour récupérer les logs
package usecase

import (
	"context"
	"fmt"
	"time"
)

// GetLogsUseCase gère la logique métier pour récupérer les logs.
type GetLogsUseCase struct {
	logRepo LogRepository
}

// LogRepository interface pour accéder aux logs.
type LogRepository interface {
	GetRecent(ctx context.Context, limit int) ([]*LogEntry, error)
	Save(ctx context.Context, entry *LogEntry) error
}

// LogEntry représente une entrée de log.
type LogEntry struct {
	ID        int                    `json:"id"`
	EventType string                 `json:"event_type"`
	Message   string                 `json:"message"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
}

// NewGetLogsUseCase crée un nouveau use case.
func NewGetLogsUseCase(logRepo LogRepository) *GetLogsUseCase {
	return &GetLogsUseCase{logRepo: logRepo}
}

// Execute récupère les logs récents.
func (uc *GetLogsUseCase) Execute(ctx context.Context, limit int) ([]*LogEntry, error) {
	// Validation
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	logs, err := uc.logRepo.GetRecent(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get logs: %w", err)
	}

	return logs, nil
}
