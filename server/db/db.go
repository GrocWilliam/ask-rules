// db/db.go — Pool de connexions PostgreSQL (pgx v5)
package db

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"ask-rules-server/config"
	"ask-rules-server/models"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var Pool *pgxpool.Pool

func Connect() error {
	var err error
	Pool, err = pgxpool.New(context.Background(), config.C.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connexion PostgreSQL : %w", err)
	}
	return Pool.Ping(context.Background())
}

func Close() {
	if Pool != nil {
		Pool.Close()
	}
}

// ── Jeux ────────────────────────────────────────────────────────────────────

func ListGames(ctx context.Context) ([]models.GameWithStats, error) {
	rows, err := Pool.Query(ctx, `
		SELECT g.id, g.jeu, g.fichier, g.date_ajout,
		       g.metadata, g.statistiques, g.gameplay,
		       COALESCE(COUNT(s.id), 0) AS sections_count
		FROM games g
		LEFT JOIN sections s ON s.game_id = g.id
		GROUP BY g.id
		ORDER BY g.jeu`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var games []models.GameWithStats
	for rows.Next() {
		var g models.GameWithStats
		var metaBytes, statsBytes, gameplayBytes []byte
		if err := rows.Scan(
			&g.ID, &g.Name, &g.FilePath, &g.AddedAt,
			&metaBytes, &statsBytes, &gameplayBytes,
			&g.SectionsCount,
		); err != nil {
			return nil, err
		}
		json.Unmarshal(metaBytes, &g.Metadata)
		json.Unmarshal(statsBytes, &g.Stats)
		json.Unmarshal(gameplayBytes, &g.Gameplay)
		games = append(games, g)
	}
	return games, rows.Err()
}

func FindGame(ctx context.Context, id string) (*models.Game, error) {
	var g models.Game
	var metaBytes, statsBytes, gameplayBytes []byte
	err := Pool.QueryRow(ctx, `
		SELECT id, jeu, fichier, date_ajout, metadata, statistiques, gameplay
		FROM games WHERE id = $1`, id).
		Scan(&g.ID, &g.Name, &g.FilePath, &g.AddedAt,
			&metaBytes, &statsBytes, &gameplayBytes)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	json.Unmarshal(metaBytes, &g.Metadata)
	json.Unmarshal(statsBytes, &g.Stats)
	json.Unmarshal(gameplayBytes, &g.Gameplay)
	return &g, nil
}

func FindGameByName(ctx context.Context, name string) (*models.Game, error) {
	var g models.Game
	var metaBytes, statsBytes, gameplayBytes []byte
	err := Pool.QueryRow(ctx, `
		SELECT id, jeu, fichier, date_ajout, metadata, statistiques, gameplay
		FROM games
		WHERE lower(jeu) LIKE lower($1)
		ORDER BY jeu LIMIT 1`, "%"+name+"%").
		Scan(&g.ID, &g.Name, &g.FilePath, &g.AddedAt,
			&metaBytes, &statsBytes, &gameplayBytes)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	json.Unmarshal(metaBytes, &g.Metadata)
	json.Unmarshal(statsBytes, &g.Stats)
	json.Unmarshal(gameplayBytes, &g.Gameplay)
	return &g, nil
}

func UpsertGame(ctx context.Context, g *models.Game) error {
	metaBytes, _ := json.Marshal(g.Metadata)
	statsBytes, _ := json.Marshal(g.Stats)
	gameplayBytes, _ := json.Marshal(g.Gameplay)
	_, err := Pool.Exec(ctx, `
		INSERT INTO games (id, jeu, fichier, date_ajout, metadata, statistiques, gameplay)
		VALUES ($1, $2, $3, NOW(), $4, $5, $6)
		ON CONFLICT (id) DO UPDATE SET
			jeu          = EXCLUDED.jeu,
			fichier      = EXCLUDED.fichier,
			metadata     = EXCLUDED.metadata,
			statistiques = EXCLUDED.statistiques,
			gameplay     = EXCLUDED.gameplay`,
		g.ID, g.Name, g.FilePath, metaBytes, statsBytes, gameplayBytes)
	return err
}

func DeleteGame(ctx context.Context, id string) error {
	_, err := Pool.Exec(ctx, "DELETE FROM games WHERE id = $1", id)
	return err
}

func UpdateGameplay(ctx context.Context, id string, gameplay map[string]interface{}) error {
	b, _ := json.Marshal(gameplay)
	_, err := Pool.Exec(ctx, "UPDATE games SET gameplay=$1 WHERE id=$2", b, id)
	return err
}

// ── Sections ─────────────────────────────────────────────────────────────────

func DeleteSections(ctx context.Context, gameID string) error {
	_, err := Pool.Exec(ctx, "DELETE FROM sections WHERE game_id = $1", gameID)
	return err
}

func InsertSection(ctx context.Context, s models.Section) error {
	var embStr *string
	if len(s.Embedding) == 384 {
		var sb strings.Builder
		sb.WriteString("[")
		for i, v := range s.Embedding {
			if i > 0 {
				sb.WriteString(",")
			}
			fmt.Fprintf(&sb, "%v", v)
		}
		sb.WriteString("]")
		str := sb.String()
		embStr = &str
	}
	_, err := Pool.Exec(ctx, `
		INSERT INTO sections
			(id, game_id, titre, niveau, type_section, contenu,
			 entites, actions, resume, mecaniques, embedding,
			 page_debut, page_fin, hierarchy_path, chunk_index, total_chunks,
			 search_vector)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,
			CASE WHEN $11::text IS NULL THEN NULL ELSE $11::vector END,
			$12,$13,$14,$15,$16,
			to_tsvector('french', $6))`,
		s.ID, s.GameID, s.Title, s.Level, s.SectionType, s.Text,
		s.Entities, s.Actions, s.Summary, s.Mechanics, embStr,
		s.PageStart, s.PageEnd, s.HierarchyPath, s.ChunkIndex, s.TotalChunks)
	return err
}

// ── Recherche vectorielle ────────────────────────────────────────────────────

func VectorSearch(ctx context.Context, gameID string, embedding []float64, limit int) ([]models.ScoredSection, error) {
	if len(embedding) == 0 {
		return nil, nil
	}
	var sb strings.Builder
	sb.WriteString("[")
	for i, v := range embedding {
		if i > 0 {
			sb.WriteString(",")
		}
		fmt.Fprintf(&sb, "%v", v)
	}
	sb.WriteString("]")
	embStr := sb.String()

	rows, err := Pool.Query(ctx, `
		SELECT id, game_id, titre, type_section, contenu, resume,
		       page_debut, page_fin,
		       1 - (embedding <=> $1::vector) AS score
		FROM sections
		WHERE game_id = $3 AND embedding IS NOT NULL
		ORDER BY embedding <=> $1::vector
		LIMIT $2`,
		embStr, limit, gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanScoredSections(rows)
}

func FullTextSearch(ctx context.Context, gameID, query string, limit int) ([]models.ScoredSection, error) {
	rows, err := Pool.Query(ctx, `
		SELECT id, game_id, titre, type_section, contenu, resume,
		       page_debut, page_fin,
		       ts_rank(search_vector, plainto_tsquery('french', $1)) AS score
		FROM sections
		WHERE game_id = $3
		  AND search_vector @@ plainto_tsquery('french', $1)
		ORDER BY score DESC
		LIMIT $2`,
		query, limit, gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanScoredSections(rows)
}

func scanScoredSections(rows pgx.Rows) ([]models.ScoredSection, error) {
	var results []models.ScoredSection
	for rows.Next() {
		var s models.ScoredSection
		if err := rows.Scan(
			&s.ID, &s.GameID, &s.Title, &s.SectionType, &s.Text, &s.Summary,
			&s.PageStart, &s.PageEnd, &s.Score,
		); err != nil {
			return nil, err
		}
		results = append(results, s)
	}
	return results, rows.Err()
}

// ── Logs ─────────────────────────────────────────────────────────────────────

func InsertLog(ctx context.Context, entry models.LogEntry) error {
	metaBytes, _ := json.Marshal(entry.Metadata)
	_, err := Pool.Exec(ctx, `
		INSERT INTO logs (event_type, message, metadata)
		VALUES ($1, $2, $3)`,
		entry.EventType, entry.Message, metaBytes)
	return err
}

func GetRecentLogs(ctx context.Context, limit int) ([]models.LogEntry, error) {
	rows, err := Pool.Query(ctx, `
		SELECT id, event_type, message, metadata, created_at
		FROM logs ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []models.LogEntry
	for rows.Next() {
		var l models.LogEntry
		var metaBytes []byte
		if err := rows.Scan(&l.ID, &l.EventType, &l.Message, &metaBytes, &l.CreatedAt); err != nil {
			return nil, err
		}
		json.Unmarshal(metaBytes, &l.Metadata)
		logs = append(logs, l)
	}
	return logs, rows.Err()
}
