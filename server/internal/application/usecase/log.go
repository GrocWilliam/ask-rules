// application/usecase/log.go — Journal des événements (table logs)
package usecase

import (
	"context"
	"time"
)

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
