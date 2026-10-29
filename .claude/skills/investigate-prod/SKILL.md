---
name: investigate-prod
description: Diagnose a bad, missing or slow answer from the deployed app using the ask-rules MCP server (db_query, llm_answer_rule). Use when the user reports a wrong rule, a wrong page, a refusal, "no sections found", or wants to know what players are asking.
---

# Investigate a production answer

The goal is to classify the failure as **indexing**, **retrieval**, **context budget** or **LLM**, then hand off to the right fix. Use the `ask-rules` MCP tools; they read the production database and call the production LLM.

Read only. Never run `go run ./cmd/server` or `cmd/eval -index` with the default `.env`: its `DATABASE_URL` is production, and the server runs migrations at startup.

## 1. Find the question

Only `ask_question` events are logged. The answer itself is **not** stored, so get it from the user or replay it (step 4).

```sql
SELECT created_at, metadata->>'game' AS game, metadata->>'question' AS question,
       metadata->>'model' AS model, metadata->>'tokens_input' AS tokens_in,
       metadata->>'history_turns' AS turns
FROM logs WHERE event_type = 'ask_question'
ORDER BY created_at DESC LIMIT 50
```

Other event types: `SELECT event_type, count(*) FROM logs GROUP BY 1`. A `feedback` table exists in production but no code or migration manages it, so treat its contents as unreliable.

## 2. Check the game is indexed properly

```sql
SELECT g.id, g.jeu, g.date_update, count(s.id) AS sections,
       count(s.id) FILTER (WHERE s.embedding IS NULL) AS no_embedding,
       count(s.id) FILTER (WHERE s.page_debut IS NULL) AS no_page,
       max(s.page_fin) AS last_page
FROM games g LEFT JOIN sections s ON s.game_id = g.id
WHERE g.jeu ILIKE '%<game>%' GROUP BY g.id
```

Each of these means an **indexing** problem; see the `import-game` skill:

- zero sections;
- sections missing embeddings;
- `last_page` far below the rulebook's page count.

## 3. Find the sections that should answer it

Search the rulebook text the way the full-text half of the hybrid search does:

```sql
SELECT id, titre, hierarchy_path, page_debut, page_fin, chunk_index, total_chunks,
       length(contenu) AS len,
       ts_rank(search_vector, plainto_tsquery('french', '<key words>')) AS rank
FROM sections WHERE game_id = '<id>'
  AND search_vector @@ plainto_tsquery('french', '<key words>')
ORDER BY rank DESC LIMIT 10
```

Also try `contenu ILIKE '%<exact phrase from the rulebook>%'`, then read the matching rows' `contenu`. Possible results:

- **The rule isn't in any section.** The extraction or chunking lost it, so the fault is **indexing** (`internal/index/pipeline.go`).
- **The rule is split across two chunks**, or sits under the wrong `hierarchy_path`. That's a **chunking** fault.
- **The rule sits in a clean section.** Continue to step 4.

The vector half (E5 embeddings, `VECTOR_MIN_SCORE` 0.72) can't run through MCP. For the exact retrieval, add the question to the eval dataset and run it on the eval database (`eval-rag` skill, `-run <question> -v`).

## 4. Replay the LLM with the right context

Build the context the way `ask.buildContext` does: group sections by chapter, then number them.

```
Chapitre : <hierarchy_path>

Section 1: <titre> (p. <page_debut>[-<page_fin>])
<contenu>
```

Pass it to `llm_answer_rule` with the player's question (plus `history` for a follow-up):

- **Wrong answer with the right sections:** an **LLM or prompt** fault; see the `tune-prompt` skill.
- **Right answer with the right sections:** production didn't send those sections. That's **retrieval** (`internal/search`, `nlp`, `VECTOR_MIN_SCORE`, the 6-result limit) or the **context budget** (`maxContextRunes` 4500 in `internal/ask/ask.go` dropping it).

## 5. Report

State the failure class, the evidence (SQL rows and the replayed answer), and the proposed fix with its file. A first question with no history is cached in memory until the backend restarts, so a fix may not show up straight away. Suggest adding the case to `server/eval/questions.json` (`eval-rag` skill) so it stays fixed.

The LLM sleeps when idle, so the first call can take about a minute while it wakes.
