# ask-rules

Assistant IA pour interroger des règles de jeux de société en français.

**Stack** : Go 1.22 · SvelteKit (SPA) · PostgreSQL + pgvector · ONNX Runtime · Redis (optionnel)

---

## Architecture

**Clean Architecture à 4 couches** :

```
ask-rules/
├── server/
│   ├── cmd/server/main.go       # Point d'entrée — DI, graceful shutdown
│   ├── internal/
│   │   ├── domain/              # Entités métier + interfaces (repository, service)
│   │   │   ├── entity/          # Game, Section, User
│   │   │   ├── repository/      # Interfaces pour persistence
│   │   │   └── service/         # Interfaces LLM, Cache, Embedder
│   │   ├── application/         # Use cases (logique métier pure)
│   │   │   └── usecase/         # AskQuestion, ImportGame, DeleteGame...
│   │   ├── infrastructure/      # Implémentations concrètes
│   │   │   ├── config/          # Chargement .env
│   │   │   ├── persistence/     # PostgreSQL repositories + migration
│   │   │   │   ├── db/          # Pool pgx v5
│   │   │   │   └── postgres/    # Implémentations repository
│   │   │   └── service/         # Embedder, LLM, Cache, Retriever, Pipeline
│   │   └── interfaces/          # Adaptateurs HTTP/CLI
│   │       └── http/
│   │           ├── handler/     # AskHandler, GamesHandler...
│   │           ├── middleware/  # AdminAuth, RateLimit
│   │           └── router/      # Chi router + timeouts par route
│   ├── .air.toml                # Live-reload Go (dev)
│   └── build/                   # SvelteKit statique (go:embed)
├── src/                         # Frontend SvelteKit (adapter-static)
│   ├── routes/
│   │   ├── +page.svelte         # Interface Q&A
│   │   ├── import/              # Import SSE (heartbeat 15s)
│   │   └── admin/               # Gestion jeux, fichiers, logs
│   └── lib/                     # Composants (SEO, Markdown, PWA)
├── static/
│   ├── service-worker.js        # PWA avec exclusions /api/*
│   └── manifest.json
├── uploads/                     # Fichiers uploadés par jeu
├── models/                      # ONNX (non versionné, téléchargé au build)
│   └── multilingual-e5-small/
├── Dockerfile                   # Multi-stage (web + Go CGO + runtime)
└── .env.example
```

**Points techniques clés** :

- **Dependency Injection** : Repositories injectés dans use cases via constructeurs
- **Chi Router** : Middleware CORS, Logger, Recoverer, timeouts par route
- **SSE Streaming** : Import avec heartbeat 15s pour éviter timeouts proxy/navigateur
- **Logging** : `[ERROR]`/`[WARN]` avec contexte (handler, use case, game name)
- **PWA** : Service Worker qui exclut `/api/*` et `text/event-stream`

---

## Prérequis

- **Go 1.22+**
- **Node.js 18+ + pnpm**
- **PostgreSQL 14+** avec l'extension `pgvector`
- **libonnxruntime.so 1.20.0** (voir ci-dessous)
- Redis (optionnel)

### Installer libonnxruntime

```bash
# Linux x64 (version 1.20.0 pour correspondre au Dockerfile)
curl -fsSL https://github.com/microsoft/onnxruntime/releases/download/v1.20.0/onnxruntime-linux-x64-1.20.0.tgz \
  | tar -xz --strip-components=2 -C /usr/local/lib '*/lib/libonnxruntime.so.1.20.0'
mv /usr/local/lib/libonnxruntime.so.1.20.0 /usr/local/lib/libonnxruntime.so
ldconfig
```

### Télécharger le modèle ONNX

```bash
./scripts/download-model.sh
# → models/multilingual-e5-small/
```

---

## Installation et lancement (développement)

```bash
# 1. Cloner et configurer
cp .env.example .env    # puis renseigner DATABASE_URL, MISTRAL_API_KEY, etc.

# 2. Installer les dépendances frontend
pnpm install

# 3. Installer Air (live-reload Go)
go install github.com/air-verse/air@latest

# 4. Builder SvelteKit + Go en une commande
pnpm run build:all

# 5. Lancer le serveur
pnpm start              # → http://localhost:3001

# 6. Development avec live-reload (optionnel)
pnpm run dev:back       # Air watch le code Go (server/)
pnpm run dev:front      # Vite dev server pour SvelteKit
```

### Scripts disponibles

| Commande             | Description                                             |
| -------------------- | ------------------------------------------------------- |
| `pnpm run build:web` | Build SvelteKit → `server/build/`                       |
| `pnpm run build:go`  | Compile le binaire Go depuis `cmd/server`               |
| `pnpm run build:all` | Les deux en séquence                                    |
| `pnpm start`         | Lance `server/ask-rules-server` (production)            |
| `pnpm run dev:back`  | Air live-reload pour Go (recompile à chaque changement) |
| `pnpm run dev:front` | Vite dev server (HMR SvelteKit)                         |

---

## Variables d'environnement

| Variable          | Défaut                                                    | Description                                   |
| ----------------- | --------------------------------------------------------- | --------------------------------------------- |
| `ENV`             | `development`                                             | Environnement (`development` ou `production`) |
| `DATABASE_URL`    | `postgresql://postgres:postgres@localhost:5432/ask-rules` | PostgreSQL                                    |
| `PORT`            | `3001`                                                    | Port d'écoute (8080 dans Docker)              |
| `MISTRAL_API_KEY` | —                                                         | LLM Mistral (prioritaire si défini)           |
| `MISTRAL_MODEL`   | `mistral-small-latest`                                    | Modèle Mistral                                |
| `OPENAI_API_KEY`  | —                                                         | LLM OpenAI                                    |
| `OPENAI_MODEL`    | `gpt-4o-mini`                                             | Modèle OpenAI                                 |
| `OLLAMA_HOST`     | `http://localhost:11434`                                  | Serveur Ollama                                |
| `OLLAMA_MODEL`    | —                                                         | Modèle Ollama (ex: `llama3`)                  |
| `ADMIN_PASSWORD`  | `admin`                                                   | Mot de passe interface admin                  |
| `REDIS_ENABLED`   | `false`                                                   | Activer le cache Redis                        |
| `REDIS_URL`       | `redis://localhost:6379`                                  | URL Redis                                     |
| `UPLOADS_DIR`     | `../uploads`                                              | Répertoire des fichiers uploadés              |
| `MODEL_PATH`      | `../models/multilingual-e5-small`                         | Chemin du modèle ONNX                         |
| `MODEL_QUANTIZED` | `true`                                                    | Télécharger le modèle int8 (118 Mo) au lieu du fp32 (470 Mo) |
| `ONNX_THREADS`    | `2`                                                       | Threads ONNX Runtime par inférence            |

**Priorité LLM** : Mistral → OpenAI → Ollama. Sans aucune clé, les réponses sont construites uniquement depuis le contexte récupéré (pas de génération).

**Note** : En `ENV=development`, le client Mistral ignore les erreurs TLS (InsecureSkipVerify).

---

## Docker

```bash
# Build (télécharge le modèle ONNX automatiquement)
docker build -t ask-rules .

# Lancement
docker run -p 8080:8080 \
  -e DATABASE_URL=postgres://user:pass@host:5432/db \
  -e MISTRAL_API_KEY=your-key \
  -v ./uploads:/app/uploads \
  ask-rules
```

### docker-compose (recommandé)

```yaml
services:
  app:
    image: ask-rules
    build: .
    ports:
      - '8080:8080'
    environment:
      ENV: production
      DATABASE_URL: postgres://postgres:postgres@db:5432/ask_rules
      MISTRAL_API_KEY: ${MISTRAL_API_KEY}
      ADMIN_PASSWORD: ${ADMIN_PASSWORD}
    volumes:
      - uploads:/app/uploads
    depends_on:
      db:
        condition: service_healthy

  db:
    image: ankane/pgvector:latest
    environment:
      POSTGRES_DB: ask_rules
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: postgres
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ['CMD-SHELL', 'pg_isready -U postgres']
      interval: 5s
      retries: 10

volumes:
  pgdata:
  uploads:
```

> Le schéma PostgreSQL est créé **automatiquement au démarrage** via migration idempotente. Pas besoin de script `migrate` séparé.  
> Le modèle ONNX est intégré dans l'image lors du `docker build` — pas de volume `/app/models` requis.

---

## Features techniques

### Clean Architecture

- **4 couches** : Domain (entities + interfaces) → Application (use cases) → Infrastructure (implémentations) → Interfaces (HTTP)
- **Dependency Injection** : Repositories et services injectés via constructeurs
- **Testabilité** : Interfaces mockables, use cases isolés de l'infra

### Chi Router

- Middleware : Logger, Recoverer, RequestID, RealIP, CORS
- **Timeouts par route** :
  - POST `/api/ask` : 60s
  - POST `/api/import` : **pas de timeout** (SSE stream)
  - GET `/api/games` : 30s
  - Admin routes : 10s
  - File serving : 120s

### SSE (Server-Sent Events)

- Import avec **heartbeat 15s** : évite les timeouts proxy/navigateur sur longues importations
- Thread-safe : `sync.Mutex` sur les envois concurrents
- Context cancellation : arrêt propre du heartbeat
- Frontend : ignore les événements `ping`

### Logging

- Format : `[ERROR]` / `[WARN]` avec contexte (handler, use case, game name)
- **Tous les handlers** : logs à chaque erreur HTTP
- **Use cases** : logs des erreurs métier (game not found, LLM failure, etc.)
- Aide au debugging : nom du jeu, opération, erreur originale

### PWA & Service Worker

- Cache-first strategy pour assets statiques
- **Exclusions** :
  - Routes `/api/*` → bypass service worker
  - Header `Accept: text/event-stream` → bypass (SSE)
- Version : `reglomatic-v2`

### Migration automatique

- Exécutée au démarrage (`db.Migrate()`)
- Idempotente : `CREATE TABLE IF NOT EXISTS`, `CREATE INDEX IF NOT EXISTS`
- Extensions : `pgvector`, `pg_trgm` (full-text search)

---

## API

### Publique

| Méthode | Route                      | Description                   |
| ------- | -------------------------- | ----------------------------- |
| `POST`  | `/api/ask`                 | Poser une question sur un jeu |
| `GET`   | `/api/games`               | Lister les jeux               |
| `GET`   | `/api/games/{id}`          | Détail d'un jeu               |
| `GET`   | `/files/{slug}/{filename}` | Servir un fichier uploadé     |

**Body `/api/ask`** :

```json
{ "question": "Comment gagner ?", "gameName": "Catan" }
```

### Admin (cookie `admin_session` requis)

| Méthode  | Route                                | Description                   |
| -------- | ------------------------------------ | ----------------------------- |
| `POST`   | `/api/admin/login`                   | Connexion                     |
| `POST`   | `/api/admin/logout`                  | Déconnexion                   |
| `GET`    | `/api/admin/check`                   | Vérifier la session           |
| `GET`    | `/api/admin/games`                   | Lister les jeux               |
| `POST`   | `/api/admin/games`                   | Créer / mettre à jour un jeu  |
| `DELETE` | `/api/admin/games/{id}`              | Supprimer un jeu              |
| `GET`    | `/api/admin/files`                   | Lister les fichiers uploadés  |
| `DELETE` | `/api/admin/files/{slug}/{filename}` | Supprimer un fichier          |
| `GET`    | `/api/admin/logs`                    | Logs récents (`?limit=N`)     |
| `POST`   | `/api/import`                        | Importer un fichier (SSE)     |
| `POST`   | `/api/admin/reprocess`               | Retraiter un jeu (SSE)        |
| `POST`   | `/api/admin/reprocess-all`           | Retraiter tous les jeux (SSE) |

---

## Embeddings

Le service `internal/infrastructure/service/onnx_embedder_adapter.go` charge le modèle **multilingual-e5-small** directement via `onnxruntime_go` (CGO). Aucun serveur Python requis.

- **Dimensions** : 384, jusqu'à 512 tokens
- **Langue** : multilingue (optimal pour le français)
- **Modèle quantifié int8** utilisé en priorité s'il est présent (`onnx/model_quantized.onnx`)
- **Préfixes E5** : `query: ` pour les questions (`Embed`), `passage: ` pour les sections indexées (`EmbedPassage`)
- **Session ONNX** : shapes dynamiques (tensors à la taille réelle du texte), arène mémoire désactivée, libérée après 5 min d'inactivité
- **Repository pattern** : Interface `EmbedderService` dans domain, implémentation dans infrastructure

Si `libonnxruntime.so` ou le modèle est absent au démarrage, le serveur continue sans embeddings (recherche full-text uniquement).

---

## Recherche hybride

`internal/domain/service/retriever/retriever.go` combine :

1. **Recherche vectorielle** — cosinus via `pgvector` (index HNSW)
2. **Full-text search** — `websearch_to_tsquery('french', 'terme1 or terme2 ...')` sur `tsvector` pondéré (titre A, hierarchy_path B, contenu C)
3. **Fusion RRF** (Reciprocal Rank Fusion) — reclassement des deux listes par rang uniquement avec `k=60`

### Évaluer la qualité du retrieval

`server/cmd/eval` mesure Hit@k et MRR sur un jeu de questions annotées (`server/eval/questions.json`).
Chaque question liste des extraits du livret attendus dans les sections renvoyées.

```bash
cd server
# Base dédiée : -index supprime et recrée les sections des jeux du dataset
DATABASE_URL=postgresql://postgres:postgres@localhost:5433/askrules_eval go run ./cmd/eval -index -v
```

Relancer l'évaluation avant/après toute modification du chunking, du modèle ou du retriever.

Les repositories PostgreSQL gèrent les requêtes SQL (separation of concerns), le retriever orchestre la fusion.

---

## Interface d'administration

Accessible sur `/admin` (mot de passe via `ADMIN_PASSWORD`) :

- **`/admin/games`** — liste des jeux avec nombre de sections, retraitement individuel ou global (suivi SSE temps réel)
- **`/admin/files`** — fichiers uploadés groupés par jeu, suppression
- **`/admin/logs`** — journal des événements groupé par date
- **`/import`** — import de nouveaux fichiers avec progression live (SSE)
