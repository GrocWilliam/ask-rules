# ask-rules

Assistant IA pour interroger des règles de jeux de société en français.

**Stack** : Go 1.22 · SvelteKit (SPA) · PostgreSQL + pgvector · ONNX Runtime · Redis (optionnel)

---

## Architecture

```
ask-rules/
├── server/                      # Binaire Go — sert tout (API + frontend)
│   ├── main.go                  # Démarrage, migration auto, graceful shutdown
│   ├── config/config.go         # Variables d'environnement
│   ├── db/
│   │   ├── db.go                # Pool pgx v5, queries
│   │   └── migrate.go           # Migration DDL idempotente (lancée au démarrage)
│   ├── embedder/embedder.go     # BERT local via onnxruntime_go (384 dims)
│   ├── handlers/                # Handlers HTTP Chi
│   │   ├── ask.go               # POST /api/ask
│   │   ├── games.go             # CRUD jeux
│   │   ├── import.go            # ImportSSE, ReprocessGame, ReprocessAll (SSE)
│   │   ├── files.go             # ServeFile, ListFiles, DeleteFile
│   │   ├── admin.go             # Login / Logout / Check
│   │   └── logs.go              # GET /api/admin/logs
│   ├── pipeline/                # Extraction texte → chunks → embeddings
│   ├── retriever/               # Hybrid search (vecteur + full-text, RRF)
│   ├── nlp/nlp.go               # Stopwords FR (~320), GAME_NOUNS (~200), mécaniques
│   ├── llm/llm.go               # Clients Mistral / OpenAI / Ollama
│   ├── cache/cache.go           # Redis ou in-memory (SHA-256 key, TTL 24h)
│   ├── router/router.go         # Chi — routes publiques, admin, SPA fallback
│   ├── middleware/              # Auth cookie, rate limiting
│   └── build/                  # SvelteKit statique (go:embed)
├── src/                         # Frontend SvelteKit (adapter-static)
│   ├── routes/
│   │   ├── +page.svelte         # Interface de question/réponse
│   │   ├── import/              # Import de fichiers (SSE temps réel)
│   │   └── admin/               # Gestion jeux, fichiers, logs
│   └── lib/                     # Composants (SEO, Markdown, PWA…)
├── uploads/                     # Fichiers uploadés (un sous-dossier par jeu)
├── models/                      # Modèle ONNX (non versionné)
│   └── multilingual-e5-small/
├── Dockerfile                   # Multi-stage : web-builder → go-builder → runtime
└── docs/                        # Guides techniques
```

---

## Prérequis

- **Go 1.22+**
- **Node.js 18+ + pnpm**
- **PostgreSQL 14+** avec l'extension `pgvector`
- **libonnxruntime.so 1.13.0** (voir ci-dessous)
- Redis (optionnel)

### Installer libonnxruntime

```bash
# Linux x64
curl -fsSL https://github.com/microsoft/onnxruntime/releases/download/v1.13.0/onnxruntime-linux-x64-1.13.0.tgz \
  | tar -xz --strip-components=2 -C /usr/local/lib '*/lib/libonnxruntime.so.1.13.0'
mv /usr/local/lib/libonnxruntime.so.1.13.0 /usr/local/lib/libonnxruntime.so
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

# 3. Builder SvelteKit + Go en une commande
pnpm run build:all

# 4. Lancer le serveur
pnpm start              # → http://localhost:3001
```

### Scripts disponibles

| Commande | Description |
|---|---|
| `pnpm run build:web` | Build SvelteKit → `server/build/` |
| `pnpm run build:go` | Compile le binaire Go |
| `pnpm run build:all` | Les deux en séquence |
| `pnpm start` | Lance `server/ask-rules-server` |

---

## Variables d'environnement

| Variable | Défaut | Description |
|---|---|---|
| `DATABASE_URL` | `postgresql://postgres:postgres@localhost:5432/ask-rules` | PostgreSQL |
| `PORT` | `3001` | Port d'écoute |
| `MISTRAL_API_KEY` | — | LLM Mistral (prioritaire si défini) |
| `MISTRAL_MODEL` | `mistral-small-latest` | Modèle Mistral |
| `OPENAI_API_KEY` | — | LLM OpenAI |
| `OPENAI_MODEL` | `gpt-4o-mini` | Modèle OpenAI |
| `OLLAMA_HOST` | `http://localhost:11434` | Serveur Ollama |
| `OLLAMA_MODEL` | — | Modèle Ollama (ex: `llama3`) |
| `ADMIN_PASSWORD` | `admin` | Mot de passe interface admin |
| `REDIS_ENABLED` | `false` | Activer le cache Redis |
| `REDIS_URL` | `redis://localhost:6379` | URL Redis |
| `UPLOADS_DIR` | `../uploads` | Répertoire des fichiers uploadés |
| `MODEL_PATH` | `../models/multilingual-e5-small` | Chemin du modèle ONNX |

**Priorité LLM** : Mistral → OpenAI → Ollama. Sans aucune clé, les réponses sont construites uniquement depuis le contexte récupéré (pas de génération).

---

## Docker

```bash
# Build (télécharge le modèle ONNX automatiquement)
docker build -t ask-rules .

# Lancement
docker run -p 3001:3001 \
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
      - "3001:3001"
    environment:
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
      test: ["CMD-SHELL", "pg_isready -U postgres"]
      interval: 5s
      retries: 10

volumes:
  pgdata:
  uploads:
```

> Le schéma PostgreSQL est créé **automatiquement au démarrage** (`db.Migrate()`). Pas besoin de lancer de script `migrate` séparément.
> Le modèle ONNX est intégré dans l'image lors du `docker build` — pas de volume `/app/models` requis.

---

## API

### Publique

| Méthode | Route | Description |
|---|---|---|
| `POST` | `/api/ask` | Poser une question sur un jeu |
| `GET` | `/api/games` | Lister les jeux |
| `GET` | `/api/games/{id}` | Détail d'un jeu |
| `GET` | `/files/{slug}/{filename}` | Servir un fichier uploadé |

**Body `/api/ask`** :
```json
{ "question": "Comment gagner ?", "jeu": "Catan", "jeu_id": "optional-uuid" }
```

### Admin (cookie `admin_session` requis)

| Méthode | Route | Description |
|---|---|---|
| `POST` | `/api/admin/login` | Connexion |
| `POST` | `/api/admin/logout` | Déconnexion |
| `GET` | `/api/admin/check` | Vérifier la session |
| `GET` | `/api/admin/games` | Lister les jeux |
| `POST` | `/api/admin/games` | Créer / mettre à jour un jeu |
| `DELETE` | `/api/admin/games/{id}` | Supprimer un jeu |
| `GET` | `/api/admin/files` | Lister les fichiers uploadés |
| `DELETE` | `/api/admin/files/{slug}/{filename}` | Supprimer un fichier |
| `GET` | `/api/admin/logs` | Logs récents (`?limit=N`) |
| `POST` | `/api/import` | Importer un fichier (SSE) |
| `POST` | `/api/admin/reprocess` | Retraiter un jeu (SSE) |
| `POST` | `/api/admin/reprocess-all` | Retraiter tous les jeux (SSE) |

---

## Embeddings

Le module `server/embedder/` charge le modèle **multilingual-e5-small** directement via `onnxruntime_go` (CGO). Aucun serveur Python requis.

- **Dimensions** : 384
- **Langue** : multilingue (optimal pour le français)
- **Session** : tensors pré-alloués, `copy()` avant chaque `Run()` — thread-safe

Si `libonnxruntime.so` ou le modèle est absent au démarrage, le serveur continue sans embeddings (recherche full-text uniquement).

---

## Recherche hybride

`server/retriever/` combine :
1. **Recherche vectorielle** — cosinus via `pgvector` (index HNSW)
2. **Full-text search** — `plainto_tsquery('french', ...)` sur `tsvector` pondéré (titre A, hierarchy_path B, contenu C)
3. **Fusion RRF** (Reciprocal Rank Fusion) — reclassement des deux listes

---

## Interface d'administration

Accessible sur `/admin` (mot de passe via `ADMIN_PASSWORD`) :

- **`/admin/games`** — liste des jeux avec nombre de sections, retraitement individuel ou global (suivi SSE temps réel)
- **`/admin/files`** — fichiers uploadés groupés par jeu, suppression
- **`/admin/logs`** — journal des événements groupé par date
- **`/import`** — import de nouveaux fichiers avec progression live (SSE)
