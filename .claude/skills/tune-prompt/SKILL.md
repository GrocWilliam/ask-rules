---
name: tune-prompt
description: Change the arbiter prompt, message layout or generation settings in server/internal/llm/client.go for the small self-hosted model (Gemma 3). Use when answers start with "Ok,", copy examples, invent rules or pages, answer only "Oui", answer in English, ramble, or when switching the LLM model.
---

# Tune the LLM prompt

The prompt in `server/internal/llm/client.go` (`systemPrompt`, `buildMessages`) targets a **very small model**: Gemma 3, 1B to 4B. llama.cpp serves whatever model it loaded and ignores `LLM_MODEL`; ask the LLM (`llm_chat`) or read the `model` field of a response to know which one is live.

## Lessons already learned: don't undo them

Each of these is backed by a comment in the code. Keep them unless an eval shows the reason no longer holds.

- **No example answers in the prompt.** When a question confuses it, the model copies examples word for word.
- **No imperative instruction in the user message.** The model replies "Ok, …". The "only source" reminder lives in the excerpts' header.
- **A single user message, even in a conversation.** Replaying user/assistant turns made the model imitate earlier answers until it replied a bare "Oui". Only previous questions and the last answer are recalled.
- **Short, numbered rules**, in French.
- **`fillerOpening`** strips "Ok, / D'accord…" in case the model ignores rule 6. Extend the regex rather than adding more prompt text.
- **Generation settings:**

  | Setting               | Value | Why                                                                |
  | --------------------- | ----- | ------------------------------------------------------------------ |
  | `llmMaxTokens`        | 400   | About twice the 150-word limit; also cuts off loops                |
  | `llmFrequencyPenalty` | 0.3   | Low, so normal repeated words ("tuile", "joueur") aren't penalised |
  | temperature           | 0.3   |                                                                    |

- **Excerpts grouped by chapter** (`ask.buildContext`): the model must never mix two chapters.

## Workflow

1. **Collect failing cases.** Take them from the user, the `investigate-prod` skill, or `cmd/eval -llm -v`. Keep the exact question and context.
2. **Baseline.** Call `llm_answer_rule` with each case and note the answers.
3. **Edit.** Change `systemPrompt`, `buildMessages` or the constants. Keep the diff small, and change one thing at a time.
4. **Reload the MCP server** (`/mcp` → reconnect `ask-rules`). It runs `go run`, so `llm_answer_rule` now uses **your local prompt against the deployed model**.
5. **Retry** the same cases, and also 5 to 10 questions that already worked, from `server/eval/questions.json`, to catch regressions. Small models are noisy: run each important case 2 or 3 times.
6. **Measure:** `DATABASE_URL=<eval db> go run ./cmd/eval -llm -v` from `server/` (`eval-rag` skill), before and after. Never use the default `.env` database.
7. **Run** `go test ./internal/llm/`. `client_test.go` checks the `buildMessages` layout; update a test only when the layout change is intended.

## Report

Show before/after answers for each case, plus the eval LLM scores, then the diff. Add every case you fixed to `server/eval/questions.json`. If you change the targeted model, update "Choix du modèle" in the README.
