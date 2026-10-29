---
name: import-game
description: Import a new board game rulebook, add files to a game, reprocess or reindex games, and check the resulting sections. Use when the user wants to add a game, a rulebook was badly extracted or chunked, after changing internal/index or nlp, or after changing the embedding model.
---

# Import or reindex a game

Indexing runs on the backend: PDF → `pdftotext` → chapters and chunks → keywords and mechanics (`nlp`) → E5 embeddings → `sections`. Supported files are `.pdf`, `.txt` and `.md`, up to 50 MB per request. Files are stored under `UPLOADS_DIR/<slug>/`.

Importing or reprocessing **writes to production**. Confirm the game name, the file and the mode with the user before calling the production API. To experiment, use a local backend with a local database (see the `eval-rag` skill), never the `.env` `DATABASE_URL`.

## Choose the operation

| Need                                                         | Operation                                            |
| ------------------------------------------------------------ | ---------------------------------------------------- |
| New game, or replace a game's rulebook                       | Import with `mode=replace` (default)                 |
| Add a file to an existing game (expansion, FAQ)              | Import with `mode=merge`                             |
| Same files, but indexing code changed (chunking, nlp, pages) | Reprocess the game                                   |
| Indexing or embedding model changed for every game           | Reprocess all, optionally with `{"game_ids": [...]}` |

Replace mode is safe: old sections stay until the new indexing succeeds (`index/reindex.go`). During reindexing, questions see both old and new sections.

## Doing it

The easy way is the admin UI: `/admin` (password `ADMIN_PASSWORD`), then `/import` or "retraiter" in `/admin/games`. Progress streams live.

With curl (`$APP` = the public URL; all of these stream SSE progress):

```bash
curl -c /tmp/ar.cookies -H 'Content-Type: application/json' \
  -d '{"password":"…"}' "$APP/api/admin/login"

# Import a file (fields: gameName, fichier, mode)
curl -N -b /tmp/ar.cookies -F gameName='Catan' -F mode=replace \
  -F fichier=@regles.pdf "$APP/api/import"

# Import from a URL
curl -N -b /tmp/ar.cookies -F gameName='Catan' -F importMode=url \
  -F url='https://…/regles.pdf' "$APP/api/import"

# Reprocess one game / several games
curl -N -b /tmp/ar.cookies -X POST "$APP/api/admin/games/<id>/reprocess"
curl -N -b /tmp/ar.cookies -H 'Content-Type: application/json' \
  -d '{"game_ids":["<id>"]}' "$APP/api/admin/reprocess-all"
```

A reprocess is a tracked job. If the stream drops, check `GET /api/admin/jobs/<id>`; the work carries on server-side.

## Check the result

Use the `ask-rules` MCP `db_query` tool:

```sql
-- Coverage
SELECT count(*) AS sections, min(page_debut), max(page_fin),
       count(*) FILTER (WHERE embedding IS NULL) AS no_embedding,
       count(*) FILTER (WHERE page_debut IS NULL) AS no_page,
       avg(length(contenu))::int AS avg_len, max(length(contenu)) AS max_len
FROM sections WHERE game_id = '<id>';

-- Structure: chapters and chunk splits
SELECT hierarchy_path, titre, type_section, page_debut, chunk_index, total_chunks, length(contenu)
FROM sections WHERE game_id = '<id>' ORDER BY page_debut, chunk_index;
```

Look for:

- the last page far below the rulebook's length (lost text);
- many sections with no page;
- garbage `contenu`, meaning two-column layouts mixed up or broken hyphenation (open a few rows);
- chapters wrongly named or flattened (`hierarchy_path`);
- huge chunks;
- `type_section` or `mecaniques` that look wrong.

Then ask 3 to 5 real questions in the app, or replay them with `llm_answer_rule`. If extraction or chunking is wrong, the fix belongs in `internal/index/pipeline.go`. Validate it with the `eval-rag` skill (`-index`) before reprocessing production.

For a game you'll want to measure, add it and a few annotated questions to `server/eval/questions.json`.
