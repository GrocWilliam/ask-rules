// db/db.go — Pool de connexions PostgreSQL (pgx v5)
package db

import (
	"context"
	"fmt"
	"time"

	"ask-rules-server/internal/infrastructure/config"

	"github.com/jackc/pgx/v5/pgxpool"
)

var Pool *pgxpool.Pool

// Connect établit la connexion au pool PostgreSQL.
func Connect() error {
	poolConfig, err := pgxpool.ParseConfig(config.C.DatabaseURL)
	if err != nil {
		return fmt.Errorf("config PostgreSQL invalide : %w", err)
	}

	// Optimisation RAM : limiter le nombre de connexions
	// Pour un backend API simple, 2-4 connexions suffisent largement
	poolConfig.MaxConns = 4 // Au lieu de ~10-20 par défaut
	poolConfig.MinConns = 1 // Minimum de connexions à maintenir
	poolConfig.MaxConnIdleTime = 5 * time.Minute

	Pool, err = pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return fmt.Errorf("connexion PostgreSQL : %w", err)
	}
	return Pool.Ping(context.Background())
}

// Close ferme le pool de connexions.
func Close() {
	if Pool != nil {
		Pool.Close()
	}
}
