---
name: eval-rag
description: Measure retrieval and answer quality with server/cmd/eval on a dedicated eval database. Use before and after any change to chunking (internal/index), search (internal/search), nlp, the embedding model, VECTOR_MIN_SCORE, the context budget or the LLM prompt, or when the user asks whether answers got better or worse.
---

# Evaluate the RAG pipeline

`server/cmd/eval` scores `server/eval/questions.json`:

- **retrieval**: Hit@k and MRR, i.e. does a returned section contain one of the `expected` excerpts?
- **page accuracy**;
- **LLM answers** (with `-llm`): expected facts present, a correct page cited, no refusal when the answer exists, a refusal when it doesn't.

## Never run against production

`.env` sets `DATABASE_URL` to the **production Railway database**. `-index` deletes and recreates the sections of every game in the dataset, so never run it, or `mage eval` (which uses `-index`), without overriding `DATABASE_URL`. Always prefix the command with a local eval database URL.

## 1. Start the eval database (once)

```bash
podman run -d --name askrules-eval -p 5433:5432 \
  -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=askrules_eval \
  docker.io/pgvector/pgvector:pg16
# Later sessions: podman start askrules-eval
export EVAL_DB=postgresql://postgres:postgres@localhost:5433/askrules_eval
```

Prerequisites for indexing:

- `libonnxruntime.so` (see the README);
- the model in `models/multilingual-e5-small/` (`./scripts/download-model.sh`);
- `pdftotext` (poppler-utils);
- the PDFs listed under `games[].files`, which are paths relative to `UPLOADS_DIR`.

If a PDF is missing, say so and stop. Don't drop the game from the dataset.

## 2. Baseline, then change, then compare

Run everything from `server/`:

```bash
# Index (needed the first time, and after any chunking/nlp/embedding change)
DATABASE_URL=$EVAL_DB go run ./cmd/eval -index -v -out /tmp/eval-before.json

# Retrieval only, on the existing index (fast)
DATABASE_URL=$EVAL_DB go run ./cmd/eval -v

# Also score the LLM answers (calls the LLM_* endpoint; the Railway LLM may take ~1 min to wake)
DATABASE_URL=$EVAL_DB go run ./cmd/eval -llm -v -out /tmp/eval-before.json
```

Other flags:

| Flag                       | Use                                                            |
| -------------------------- | -------------------------------------------------------------- |
| `-run <regexp>`            | Only the matching questions                                    |
| `-model ../models/<other>` | Compare another embedding model in memory, without reindexing  |
| `-min-score <f>`           | Try another vector similarity threshold (negative disables it) |
| `-dataset <file>`          | Use another dataset                                            |

Which steps to run after a change:

- **Chunking, nlp or embedding changes:** re-run with `-index`, since sections must be rebuilt.
- **Search, context-budget or prompt changes:** skip `-index`.

Run the same flags before and after the change, keeping both `-out` files.

## 3. Report

Give a before/after table: Hit@k, MRR, pages, and the LLM scores if run. Name each question that changed (newly passing or newly failing) and quote the `-v` detail for regressions. A change that lowers any metric needs an explanation, or a rollback, before it's committed.

## Adding a question to the dataset

Every bad production answer that gets fixed should become a dataset question:

```json
{
  "game": "Defcon 1",
  "question": "…",
  "expected": ["exact excerpt from the rulebook"],
  "answer": ["3|trois"]
}
```

- `expected`: excerpts matched against section text, ignoring case, spaces and typographic apostrophes. Leave it empty when the rulebook doesn't contain the answer; the LLM must then refuse.
- `answer`: facts the answer must contain, matched as whole words, with variants separated by `|`.
- `history`: optional, for follow-up questions.
