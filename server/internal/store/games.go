// store/games.go — Implémentation PostgreSQL
package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Game représente un jeu de société.
// Correspond à la table `games` en base de données.
type Game struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`       // colonne PG: jeu
	FilePath  string                 `json:"file_path"`  // colonne PG: fichier
	AddedAt   time.Time              `json:"added_at"`   // colonne PG: date_ajout
	UpdatedAt time.Time              `json:"updated_at"` // colonne PG: date_update
	Metadata  map[string]interface{} `json:"metadata"`
	Stats     map[string]interface{} `json:"stats"` // colonne PG: statistiques
	Gameplay  map[string]interface{} `json:"gameplay"`
}

// Files retourne les chemins (relatifs à UPLOADS_DIR) des fichiers du jeu,
// rangés dans Stats["files"].
func (g *Game) Files() []string {
	return toStringSlice(g.Stats["files"])
}

// toStringSlice convertit un []interface{} (issu de JSON/pgx) en []string.
func toStringSlice(v interface{}) []string {
	if v == nil {
		return nil
	}
	if s, ok := v.([]string); ok {
		return s
	}
	if raw, ok := v.([]interface{}); ok {
		result := make([]string, 0, len(raw))
		for _, item := range raw {
			if str, ok := item.(string); ok {
				result = append(result, str)
			}
		}
		return result
	}
	return nil
}

// GameWithStats est un jeu avec des statistiques (nombre de sections).
type GameWithStats struct {
	Game
	SectionsCount int `json:"sections_count"`
}

// GameRepository définit les opérations sur les jeux.
// Cette interface est indépendante de l'implémentation (PostgreSQL, MongoDB, etc.)
type GameRepository interface {
	// FindByID retourne un jeu par son ID.
	// Retourne ErrGameNotFound si le jeu n'existe pas.
	FindByID(ctx context.Context, id string) (*Game, error)

	// FindByName retourne un jeu par son nom (recherche insensible à la casse).
	// Retourne ErrGameNotFound si aucun jeu ne correspond.
	FindByName(ctx context.Context, name string) (*Game, error)

	// List retourne tous les jeux avec leur compte de sections.
	List(ctx context.Context) ([]*GameWithStats, error)

	// Save crée ou met à jour un jeu.
	// Si l'ID existe déjà, met à jour, sinon crée.
	Save(ctx context.Context, game *Game) error

	// Delete supprime un jeu et toutes ses sections.
	Delete(ctx context.Context, id string) error

	// UpdateGameplay met à jour uniquement le champ gameplay d'un jeu.
	UpdateGameplay(ctx context.Context, id string, gameplay map[string]interface{}) error

	// Touch met à jour date_update (règles réindexées) : les réponses en
	// cache, dont la clé en dépend, ne sont plus servies.
	Touch(ctx context.Context, id string) error
}

// gameRepository implémente GameRepository avec PostgreSQL.
type gameRepository struct {
	pool *pgxpool.Pool
}

// NewGameRepository crée un nouveau repository PostgreSQL pour les jeux.
func NewGameRepository(pool *pgxpool.Pool) GameRepository {
	return &gameRepository{pool: pool}
}

// FindByID retourne un jeu par son ID.
func (r *gameRepository) FindByID(ctx context.Context, id string) (*Game, error) {
	query := `SELECT id, jeu, fichier, date_ajout, date_update, metadata, statistiques, gameplay 
	          FROM games WHERE id = $1`

	var game Game
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&game.ID,
		&game.Name,
		&game.FilePath,
		&game.AddedAt,
		&game.UpdatedAt,
		&game.Metadata,
		&game.Stats,
		&game.Gameplay,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrGameNotFound
		}
		return nil, err
	}

	return &game, nil
}

// FindByName retourne un jeu par son nom (insensible à la casse).
func (r *gameRepository) FindByName(ctx context.Context, name string) (*Game, error) {
	query := `SELECT id, jeu, fichier, date_ajout, date_update, metadata, statistiques, gameplay 
	          FROM games WHERE LOWER(jeu) = LOWER($1)`

	var game Game
	err := r.pool.QueryRow(ctx, query, name).Scan(
		&game.ID,
		&game.Name,
		&game.FilePath,
		&game.AddedAt,
		&game.UpdatedAt,
		&game.Metadata,
		&game.Stats,
		&game.Gameplay,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrGameNotFound
		}
		return nil, err
	}

	return &game, nil
}

// List retourne tous les jeux avec leur compte de sections.
func (r *gameRepository) List(ctx context.Context) ([]*GameWithStats, error) {
	query := `
		SELECT g.id, g.jeu, g.fichier, g.date_ajout, g.date_update, g.metadata, g.statistiques, g.gameplay, 
		       COALESCE(COUNT(s.id), 0) as sections_count
		FROM games g
		LEFT JOIN sections s ON g.id = s.game_id
		GROUP BY g.id, g.jeu, g.fichier, g.date_ajout, g.date_update, g.metadata, g.statistiques, g.gameplay
		ORDER BY g.jeu
	`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var games []*GameWithStats
	for rows.Next() {
		var gws GameWithStats
		err := rows.Scan(
			&gws.ID,
			&gws.Name,
			&gws.FilePath,
			&gws.AddedAt,
			&gws.UpdatedAt,
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
func (r *gameRepository) Save(ctx context.Context, game *Game) error {
	query := `
		INSERT INTO games (id, jeu, fichier, date_ajout, date_update, metadata, statistiques, gameplay)
		VALUES ($1, $2, $3, $4, NOW(), $5, $6, $7)
		ON CONFLICT (id) DO UPDATE SET
			jeu          = EXCLUDED.jeu,
			fichier      = EXCLUDED.fichier,
			date_update  = NOW(),
			metadata     = EXCLUDED.metadata,
			statistiques = EXCLUDED.statistiques,
			gameplay     = EXCLUDED.gameplay
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
func (r *gameRepository) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM games WHERE id = $1`
	result, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrGameNotFound
	}

	return nil
}

// UpdateGameplay met à jour uniquement le champ gameplay.
func (r *gameRepository) UpdateGameplay(ctx context.Context, id string, gameplay map[string]interface{}) error {
	query := `UPDATE games SET gameplay = $1 WHERE id = $2`

	result, err := r.pool.Exec(ctx, query, gameplay, id)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrGameNotFound
	}

	return nil
}

// Touch met à jour date_update (règles réindexées).
func (r *gameRepository) Touch(ctx context.Context, id string) error {
	result, err := r.pool.Exec(ctx, `UPDATE games SET date_update = NOW() WHERE id = $1`, id)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrGameNotFound
	}

	return nil
}
