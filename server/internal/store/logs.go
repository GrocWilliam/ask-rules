// store/logs.go — Repository pour les logs
package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
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

// logRepository implémente LogRepository avec PostgreSQL.
type logRepository struct {
	pool *pgxpool.Pool
}

// NewLogRepository crée un nouveau repository PostgreSQL pour les logs.
func NewLogRepository(pool *pgxpool.Pool) LogRepository {
	return &logRepository{pool: pool}
}

// GetRecent récupère les logs récents.
func (r *logRepository) GetRecent(ctx context.Context, limit int) ([]*LogEntry, error) {
	query := `
		SELECT id, event_type, message, metadata, created_at
		FROM logs
		ORDER BY created_at DESC
		LIMIT $1
	`

	rows, err := r.pool.Query(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []*LogEntry
	for rows.Next() {
		var log LogEntry
		err := rows.Scan(
			&log.ID,
			&log.EventType,
			&log.Message,
			&log.Metadata,
			&log.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		logs = append(logs, &log)
	}

	return logs, rows.Err()
}

// Save insère une nouvelle entrée de log.
func (r *logRepository) Save(ctx context.Context, entry *LogEntry) error {
	query := `
		INSERT INTO logs (event_type, message, metadata)
		VALUES ($1, $2, $3)
	`
	_, err := r.pool.Exec(ctx, query, entry.EventType, entry.Message, entry.Metadata)
	return err
}
