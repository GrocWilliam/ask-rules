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

	// Configuration optimisée pour environnement serverless :
	// - MinConns = 0 permet au pool de fermer TOUTES les connexions en cas d'inactivité
	//   et ainsi libérer le container pour mise en veille
	// - MaxConnIdleTime court pour fermer rapidement les connexions inutilisées
	// - MaxConnLifetime force la rotation des connexions pour éviter les connexions "zombies"
	poolConfig.MaxConns = 4                        // Maximum de connexions simultanées
	poolConfig.MinConns = 0                        // Permet la fermeture de toutes les connexions
	poolConfig.MaxConnIdleTime = 30 * time.Second  // Fermeture après 30s d'inactivité
	poolConfig.MaxConnLifetime = 30 * time.Minute  // Rotation des connexions toutes les 30 min
	poolConfig.HealthCheckPeriod = 1 * time.Minute // Vérification périodique des connexions

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
