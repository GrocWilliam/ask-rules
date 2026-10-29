---
name: deploy-railway
description: Prepare, verify and check a release of ask-rules (Go backend, SvelteKit/nginx frontend, llama.cpp LLM, PostgreSQL on Railway). Use before pushing a release, when changing Dockerfiles, nginx config or environment variables, or when a deploy misbehaves.
---

# Deploy to Railway

The app is deployed as several Railway services:

| Service    | Built from            | Notes                                                                                                          |
| ---------- | --------------------- | -------------------------------------------------------------------------------------------------------------- |
| backend    | `Dockerfile.backend`  | Go, CGO, ONNX Runtime and the model baked in; `poppler-utils` for PDFs; listens on 8080                        |
| frontend   | `Dockerfile.frontend` | Static SvelteKit build served by nginx, proxying `/api`, `/files` and `/health` to `BACKEND_HOST:BACKEND_PORT` |
| llama.cpp  | —                     | OpenAI-compatible API; sleeps when idle, wakes in about 40 s                                                   |
| PostgreSQL | —                     | With pgvector                                                                                                  |

Pushing, deploying and changing Railway variables are outward-facing actions. Prepare everything, then let the user push or confirm each step. If you don't know how this project triggers deploys (git push on `main`, Railway CLI…), ask.

## Before the release

1. **Backend:** `cd server && go vet ./... && go test ./...`
2. **Frontend:** `pnpm lint && pnpm format:check && pnpm build:web`
3. **Quality:** if retrieval, indexing or the prompt changed, run the `eval-rag` skill and include its before/after table.
4. **Schema:** `git diff <last release>.. -- server/internal/store/migrate.go`. New steps run automatically at the next backend start, in one transaction. Review them against the `add-migration` rules and the production row counts (`db_schema`). A failing migration stops the backend from starting.
5. **Re-indexing:** changes to `internal/index`, `nlp` or the embedding model don't touch existing sections. Plan a reprocess after the deploy (`import-game` skill).
6. **Variables:** a new or renamed variable must be set on the Railway service before the deploy, and documented in `.env.example` and the README table. Backend: `DATABASE_URL`, `LLM_BASE_URL`, `LLM_HEALTH_URL` (internal `*.railway.internal` URLs inside Railway), `ADMIN_PASSWORD`, `ASK_RATE_LIMIT_PER_MINUTE`. Frontend: `BACKEND_HOST`, `BACKEND_PORT`.
7. **Dockerfiles:** if one changed, build it locally: `podman build -f Dockerfile.backend .` (the build downloads ONNX Runtime and the model; slow).

Summarise for the user: commits included, migrations, variables to set, whether a reprocess is needed, and the eval results.

## After the deploy

1. Check `curl -fsS "$APP/health"` returns `OK`. The backend may be waking up, so retry for about 1 minute.
2. Check `curl -fsS "$APP/api/games"` lists the games.
3. Ask one real question through the app or `POST /api/ask` (`{"game": "…", "question": "…"}`). The first one can wait for the LLM to wake. This logs an `ask_question` row; that's expected.
4. Check the backend logs on Railway: the `✔` migration lines, no `[ERROR]`.
5. Use the MCP tools to confirm new questions appear in `logs` and that any new column or table exists (`db_schema`).

## Rollback

Redeploy the previous successful deployment of the affected service in Railway. Migrations are not rolled back. That's harmless when they only added things; if a step dropped or rewrote data, say so before rolling back, because the old code may expect what was removed.
