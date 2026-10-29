---
name: add-migration
description: Change the PostgreSQL schema (tables, columns, indexes, the search_vector trigger, the embedding dimension) through the idempotent steps in server/internal/store/migrate.go. Use whenever a feature needs a new column, table or index, or a change to full-text or vector search storage.
---

# Add a schema migration

There are no migration files. `store.Migrate` replays the whole `steps` list, inside a single transaction, **on every backend startup**, including every wake-up of the sleeping Railway backend.

## Rules for a step

- **Idempotent:** `CREATE … IF NOT EXISTS`, `ADD COLUMN IF NOT EXISTS`, `DROP … IF EXISTS`, `CREATE OR REPLACE FUNCTION`, `DROP TRIGGER IF EXISTS` then `CREATE TRIGGER`.
- **Appended after the step it depends on**, usually at the end of its table's block. Never edit a step that has already shipped; add a new one (see the existing "(migration existante)" steps).
- **A French label**, following the existing ones (`"colonne date_update"`).
- **Cheap on every run.** A backfill must filter on rows that still need it (`WHERE search_vector IS NULL`); an unfiltered `UPDATE` rewrote the whole table at each start and slowed wake-ups. A new `NOT NULL` column needs a `DEFAULT`.
- **Compatible with the running version.** For a moment the old backend runs on the new schema, so add before you remove. Drop a column in a later release, once no code reads it (as done for `niveau`, `entites`, `actions`).
- **Destructive steps need explicit agreement.** That covers `DROP COLUMN`, `DROP TABLE` and type changes. Ask the user first, and say what data is lost.

## Changes with knock-on effects

- **`sections` search fields** (`titre`, `hierarchy_path`, `mecaniques`, `contenu`): the trigger function, its `UPDATE OF` column list and the backfill all build the same weighted `tsvector`. Change all three together.
- **Changing the weights or the text-search configuration** (`'french'`): existing rows aren't recomputed by the filtered backfill. Plan a one-off recompute, and say how long it takes on production (row count via `db_schema`).
- **`embedding vector(384)`** is tied to multilingual-e5-small. Another embedding model means changing the dimension, rebuilding the HNSW index and reindexing every game (`import-game` skill, reprocess-all). Treat it as a project, not a step.
- **`feedback`** exists in production but not in `migrate.go`. Check with the user before managing it here.

## After writing the step

1. Update the repository in `store/<table>.go`: struct, `SELECT` and `Scan` lists, inserts. Update any `ask` or `index` code and the tests' fakes.
2. Test against a **local** database, never the `.env` one (it's production):
   ```bash
   podman start askrules-eval   # or create it: see the eval-rag skill
   cd server && DATABASE_URL=postgresql://postgres:postgres@localhost:5433/askrules_eval go run ./cmd/server
   ```
   Start it **twice**. The second start must succeed with the same `✔` lines, which proves the step is idempotent.
3. Check with `\d sections` (`psql`), then run `go vet ./... && go test ./...`.
4. Update the README if the change is user-visible (variables, API, the data described under "Recherche hybride").
5. Before deploying, read the step against the production row counts (`db_schema`). It will run inside the startup transaction on the next deploy.
