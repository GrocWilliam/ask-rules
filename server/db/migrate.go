// db/migrate.go — Migration automatique du schéma PostgreSQL au démarrage
//
// Stratégie : SQL idempotent (CREATE IF NOT EXISTS, ADD COLUMN IF NOT EXISTS,
// CREATE OR REPLACE FUNCTION) — peut être rejoué sans danger à chaque démarrage.
package db

import (
	"context"
	"fmt"
	"log"
)

// Migrate applique toutes les migrations DDL dans une transaction unique.
// Idempotent : peut être appelé à chaque démarrage.
func Migrate(ctx context.Context) error {
	tx, err := Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("migrate: begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	steps := []struct {
		label string
		sql   string
	}{
		// ── Extension pgvector ────────────────────────────────────────────────
		{
			"extension vector",
			`CREATE EXTENSION IF NOT EXISTS vector`,
		},

		// ── Table games ───────────────────────────────────────────────────────
		{
			"table games",
			`CREATE TABLE IF NOT EXISTS games (
				id           TEXT        PRIMARY KEY,
				jeu          TEXT        NOT NULL,
				fichier      TEXT        NOT NULL,
				date_ajout   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				metadata     JSONB       NOT NULL DEFAULT '{}',
				statistiques JSONB       NOT NULL DEFAULT '{}',
				gameplay     JSONB       NOT NULL DEFAULT '{}'
			)`,
		},
		{
			"colonne gameplay (migration existante)",
			`ALTER TABLE games
				ADD COLUMN IF NOT EXISTS gameplay JSONB NOT NULL DEFAULT '{}'`,
		},

		// ── Table sections ────────────────────────────────────────────────────
		{
			"table sections",
			`CREATE TABLE IF NOT EXISTS sections (
				id             TEXT        PRIMARY KEY,
				game_id        TEXT        NOT NULL REFERENCES games(id) ON DELETE CASCADE,
				titre          TEXT        NOT NULL,
				niveau         INTEGER     NOT NULL,
				type_section   TEXT        NOT NULL,
				contenu        TEXT        NOT NULL,
				entites        TEXT[]      NOT NULL DEFAULT '{}',
				actions        TEXT[]      NOT NULL DEFAULT '{}',
				resume         TEXT        NOT NULL DEFAULT '',
				mecaniques     TEXT[]      NOT NULL DEFAULT '{}',
				embedding      vector(384),
				page_debut     INTEGER,
				page_fin       INTEGER,
				hierarchy_path TEXT        NOT NULL DEFAULT '',
				chunk_index    INTEGER     NOT NULL DEFAULT 0,
				total_chunks   INTEGER     NOT NULL DEFAULT 1,
				search_vector  tsvector
			)`,
		},
		{
			"colonnes page_debut / page_fin (migration existante)",
			`ALTER TABLE sections
				ADD COLUMN IF NOT EXISTS page_debut INTEGER,
				ADD COLUMN IF NOT EXISTS page_fin   INTEGER`,
		},
		{
			"colonnes chunking hiérarchique (migration existante)",
			`ALTER TABLE sections
				ADD COLUMN IF NOT EXISTS hierarchy_path TEXT    NOT NULL DEFAULT '',
				ADD COLUMN IF NOT EXISTS chunk_index    INTEGER NOT NULL DEFAULT 0,
				ADD COLUMN IF NOT EXISTS total_chunks   INTEGER NOT NULL DEFAULT 1`,
		},
		{
			"colonne search_vector (migration existante)",
			`ALTER TABLE sections
				ADD COLUMN IF NOT EXISTS search_vector tsvector`,
		},

		// ── Table logs ────────────────────────────────────────────────────────
		{
			"table logs",
			`CREATE TABLE IF NOT EXISTS logs (
				id          SERIAL       PRIMARY KEY,
				event_type  TEXT         NOT NULL,
				message     TEXT         NOT NULL,
				metadata    JSONB,
				ip_address  TEXT,
				user_agent  TEXT,
				created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
			)`,
		},

		// ── Index sur logs ────────────────────────────────────────────────────
		{
			"index logs_event_type_idx",
			`CREATE INDEX IF NOT EXISTS logs_event_type_idx ON logs(event_type)`,
		},
		{
			"index logs_created_at_idx",
			`CREATE INDEX IF NOT EXISTS logs_created_at_idx ON logs(created_at DESC)`,
		},
		{
			"index logs_ip_address_idx",
			`CREATE INDEX IF NOT EXISTS logs_ip_address_idx ON logs(ip_address)
				WHERE ip_address IS NOT NULL`,
		},

		// ── Index HNSW (recherche vectorielle cosinus) ────────────────────────
		{
			"index HNSW sections.embedding",
			`CREATE INDEX IF NOT EXISTS sections_embedding_hnsw_idx
				ON sections USING hnsw (embedding vector_cosine_ops)
				WITH (m = 16, ef_construction = 64)`,
		},

		// ── Index GIN (full-text search) ──────────────────────────────────────
		{
			"index GIN sections.search_vector",
			`CREATE INDEX IF NOT EXISTS sections_search_vector_idx
				ON sections USING gin (search_vector)`,
		},

		// ── Backfill search_vector pour les sections existantes ───────────────
		{
			"backfill search_vector",
			`UPDATE sections
				SET search_vector =
					setweight(to_tsvector('french', coalesce(titre, '')), 'A') ||
					setweight(to_tsvector('french', coalesce(hierarchy_path, '')), 'B') ||
					setweight(to_tsvector('french', coalesce(contenu, '')), 'C')
				WHERE search_vector IS NULL`,
		},

		// ── Trigger de mise à jour automatique du search_vector ───────────────
		{
			"fonction trigger search_vector",
			`CREATE OR REPLACE FUNCTION sections_search_vector_trigger() RETURNS trigger AS $$
			BEGIN
				NEW.search_vector :=
					setweight(to_tsvector('french', coalesce(NEW.titre, '')), 'A') ||
					setweight(to_tsvector('french', coalesce(NEW.hierarchy_path, '')), 'B') ||
					setweight(to_tsvector('french', coalesce(NEW.contenu, '')), 'C');
				RETURN NEW;
			END
			$$ LANGUAGE plpgsql`,
		},
		{
			"trigger sections_search_vector_update",
			`DROP TRIGGER IF EXISTS sections_search_vector_update ON sections;
			CREATE TRIGGER sections_search_vector_update
				BEFORE INSERT OR UPDATE OF titre, hierarchy_path, contenu
				ON sections
				FOR EACH ROW
				EXECUTE FUNCTION sections_search_vector_trigger()`,
		},
	}

	for _, step := range steps {
		if _, err := tx.Exec(ctx, step.sql); err != nil {
			return fmt.Errorf("migrate [%s]: %w", step.label, err)
		}
		log.Printf("  ✔ %s", step.label)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("migrate: commit: %w", err)
	}
	return nil
}
