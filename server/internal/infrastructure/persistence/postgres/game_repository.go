// infrastructure/persistence/postgres/game_repository.go — Implémentation PostgreSQL
package postgres

import (
	"context"
	"errors"
	"time"

	"ask-rules-server/internal/domain/entity"
	"ask-rules-server/internal/domain/repository"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// GameRepositoryImpl implémente repository.GameRepository avec PostgreSQL.
type GameRepositoryImpl struct {
	pool *pgxpool.Pool
}

// NewGameRepository crée un nouveau repository PostgreSQL pour les jeux.
func NewGameRepository(pool *pgxpool.Pool) repository.GameRepository {
	return &GameRepositoryImpl{pool: pool}
}

// FindByID retourne un jeu par son ID.
func (r *GameRepositoryImpl) FindByID(ctx context.Context, id string) (*entity.Game, error) {
	query := `SELECT id, jeu, fichier, date_ajout, metadata, statistiques, gameplay 
	          FROM games WHERE id = $1`

	var game entity.Game
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&game.ID,
		&game.Name,
		&game.FilePath,
		&game.AddedAt,
		&game.Metadata,
		&game.Stats,
		&game.Gameplay,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, entity.ErrGameNotFound
		}
		return nil, err
	}

	return &game, nil
}

// FindByName retourne un jeu par son nom (insensible à la casse).
func (r *GameRepositoryImpl) FindByName(ctx context.Context, name string) (*entity.Game, error) {
	query := `SELECT id, jeu, fichier, date_ajout, metadata, statistiques, gameplay 
	          FROM games WHERE LOWER(jeu) = LOWER($1)`

	var game entity.Game
	err := r.pool.QueryRow(ctx, query, name).Scan(
		&game.ID,
		&game.Name,
		&game.FilePath,
		&game.AddedAt,
		&game.Metadata,
		&game.Stats,
		&game.Gameplay,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, entity.ErrGameNotFound
		}
		return nil, err
	}

	return &game, nil
}

// List retourne tous les jeux avec leur compte de sections.
func (r *GameRepositoryImpl) List(ctx context.Context) ([]*entity.GameWithStats, error) {
	query := `
		SELECT g.id, g.jeu, g.fichier, g.date_ajout, g.metadata, g.statistiques, g.gameplay, 
		       COALESCE(COUNT(s.id), 0) as sections_count
		FROM games g
		LEFT JOIN sections s ON g.id = s.game_id
		GROUP BY g.id, g.jeu, g.fichier, g.date_ajout, g.metadata, g.statistiques, g.gameplay
		ORDER BY g.jeu
	`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var games []*entity.GameWithStats
	for rows.Next() {
		var gws entity.GameWithStats
		err := rows.Scan(
			&gws.ID,
			&gws.Name,
			&gws.FilePath,
			&gws.AddedAt,
			&gws.Metadata,
			&gws.Stats,
			&gws.Gameplay,
			&gws.SectionsCount,
		)
		if err != nil {
			return nil, err
		}
		games = append(games, &gws)
	}

	return games, rows.Err()
}

// Save crée ou met à jour un jeu.
func (r *GameRepositoryImpl) Save(ctx context.Context, game *entity.Game) error {
	query := `
		INSERT INTO games (id, jeu, fichier, date_ajout, metadata, statistiques, gameplay)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (id) DO UPDATE SET
			jeu = EXCLUDED.jeu,
			fichier = EXCLUDED.fichier,
			metadata = EXCLUDED.metadata,
			statistiques = EXCLUDED.statistiques,
			gameplay = EXCLUDED.gameplay
	`

	if game.AddedAt.IsZero() {
		game.AddedAt = time.Now()
	}

	// Garantir que les champs JSONB ne sont jamais nil (contrainte NOT NULL)
	if game.Metadata == nil {
		game.Metadata = make(map[string]interface{})
	}
	if game.Stats == nil {
		game.Stats = make(map[string]interface{})
	}
	if game.Gameplay == nil {
		game.Gameplay = make(map[string]interface{})
	}

	_, err := r.pool.Exec(ctx, query,
		game.ID,
		game.Name,
		game.FilePath,
		game.AddedAt,
		game.Metadata,
		game.Stats,
		game.Gameplay,
	)

	return err
}

// Delete supprime un jeu et toutes ses sections (CASCADE).
func (r *GameRepositoryImpl) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM games WHERE id = $1`
	result, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return entity.ErrGameNotFound
	}

	return nil
}

// UpdateGameplay met à jour uniquement le champ gameplay.
func (r *GameRepositoryImpl) UpdateGameplay(ctx context.Context, id string, gameplay map[string]interface{}) error {
	query := `UPDATE games SET gameplay = $1 WHERE id = $2`

	result, err := r.pool.Exec(ctx, query, gameplay, id)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return entity.ErrGameNotFound
	}

	return nil
}
