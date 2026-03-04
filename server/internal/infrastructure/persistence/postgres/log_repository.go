// infrastructure/persistence/postgres/log_repository.go — Repository pour les logs
package postgres

import (
	"context"

	"ask-rules-server/internal/application/usecase"

	"github.com/jackc/pgx/v5/pgxpool"
)

// LogRepositoryImpl implémente usecase.LogRepository avec PostgreSQL.
type LogRepositoryImpl struct {
	pool *pgxpool.Pool
}

// NewLogRepository crée un nouveau repository PostgreSQL pour les logs.
func NewLogRepository(pool *pgxpool.Pool) usecase.LogRepository {
	return &LogRepositoryImpl{pool: pool}
}

// GetRecent récupère les logs récents.
func (r *LogRepositoryImpl) GetRecent(ctx context.Context, limit int) ([]*usecase.LogEntry, error) {
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

	var logs []*usecase.LogEntry
	for rows.Next() {
		var log usecase.LogEntry
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
func (r *LogRepositoryImpl) Save(ctx context.Context, entry *usecase.LogEntry) error {
	query := `
		INSERT INTO logs (event_type, message, metadata)
		VALUES ($1, $2, $3)
	`
	_, err := r.pool.Exec(ctx, query, entry.EventType, entry.Message, entry.Metadata)
	return err
}
