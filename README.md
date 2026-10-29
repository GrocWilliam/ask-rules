# ask-rules

Assistant IA pour interroger des règles de jeux de société en français.

**Stack** : Go 1.25 · SvelteKit (SPA) · PostgreSQL + pgvector · ONNX Runtime

---

## Architecture

Un serveur Go découpé en paquets par rôle, sans couches : chaque paquet fait une chose, et un handler appelle directement `store` ou `files` quand il n'y a pas de logique à porter.

```
server/
├── cmd/server/main.go   # Point d'entrée : configuration, base, services, routes
├── cmd/eval/            # Évaluation du retrieval et des réponses du LLM
├── cmd/mcp/             # Serveur MCP : base et LLM déployés pour un assistant
└── internal/
    ├── config/          # Chargement du .env
    ├── store/           # PostgreSQL : un fichier par table (games, sections, logs), migrations
    ├── files/           # Fichiers uploadés (UPLOADS_DIR)
    ├── embed/           # Embeddings ONNX (multilingual-e5-small)
    ├── llm/             # Client chat/completions : retry, cadence, réveil d'un LLM en veille
    ├── cache/           # Cache des réponses en mémoire
    ├── nlp/             # Mots-clés, type de section, mécaniques, gameplay
    ├── search/          # Recherche hybride (vectorielle + plein texte, fusion RRF)
    ├── ask/             # Question → recherche → contexte → LLM → cache
    ├── index/           # Import, découpage, indexation, réindexation, suppression des jeux
    └── web/             # Handlers HTTP, routes Chi, middlewares, sessions admin
src/                     # Frontend SvelteKit (SPA, adapter-static)
static/                  # Service worker, manifest PWA
deploy/nginx/            # nginx : sert le frontend, proxifie /api, /files et /health
Dockerfile.backend       # Serveur Go (CGO + ONNX Runtime + modèle)
Dockerfile.frontend      # Build SvelteKit servi par nginx
```

Les dépendances vont dans un seul sens : `web` → `ask`, `index` → `search`, `llm`, `embed`, `files`, `nlp` → `store` → `config`.

- **Logique** : `ask` (répondre à une question) et `index` (construire et maintenir les sections d'un jeu) ; les autres paquets sont des outils qu'ils utilisent.
- **Interfaces** : seulement là où un test a besoin d'un faux, déclarées par le paquet qui les utilise (`ask.Searcher`, `ask.LLM`, `ask.Cache`, `search.Embedder`, `index.Processor`), plus les dépôts de `store` (`GameRepository`, `SectionRepository`, `LogRepository`).
- **Câblage** : tout est construit dans `cmd/server/main.go`, sans framework d'injection.

---

## Prérequis

- **Go 1.25+**
- **Node.js 18+ + pnpm**
- **PostgreSQL 14+** avec l'extension `pgvector`
- **libonnxruntime.so 1.20.0** (voir ci-dessous)

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
cp .env.example .env    # puis renseigner DATABASE_URL, LLM_BASE_URL, etc.

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

Côté serveur, [Mage](https://magefile.org) regroupe les tâches Go (`go install github.com/magefile/mage@latest`, puis depuis `server/`) :

| Commande     | Description                                        |
| ------------ | -------------------------------------------------- |
| `mage`       | `go vet` + tests (cible par défaut)                |
| `mage dev`   | Live-reload avec Air                               |
| `mage build` | Compile `ask-rules-server`                         |
| `mage eval`  | Évaluation du retrieval (`DATABASE_URL` à définir) |
| `mage -l`    | Liste les cibles                                   |

---

## Variables d'environnement

| Variable                    | Défaut                                                    | Description                                                                                                                                                          |
| --------------------------- | --------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `ENV`                       | `development`                                             | Environnement (`development` ou `production`)                                                                                                                        |
| `DATABASE_URL`              | `postgresql://postgres:postgres@localhost:5432/ask-rules` | PostgreSQL                                                                                                                                                           |
| `PORT`                      | `3001`                                                    | Port d'écoute (8080 dans Docker)                                                                                                                                     |
| `LLM_BASE_URL`              | —                                                         | API compatible OpenAI du LLM, sans `/chat/completions` (ex : `http://llama.railway.internal:8080/v1`, `https://api.mistral.ai/v1`)                                   |
| `LLM_API_KEY`               | —                                                         | Clé d'API (optionnelle pour llama.cpp sans `--api-key`)                                                                                                              |
| `LLM_MODEL`                 | —                                                         | Modèle (optionnel pour llama.cpp)                                                                                                                                    |
| `LLM_TIMEOUT_SECONDS`       | `60`                                                      | Délai max d'une requête, tentatives comprises                                                                                                                        |
| `LLM_REQUESTS_PER_SECOND`   | `0`                                                       | Cadence max (0 = illimitée ; `1` pour l'offre gratuite Mistral). Les 429/5xx sont réessayés avec backoff                                                             |
| `LLM_HEALTH_URL`            | —                                                         | Endpoint de santé d'un LLM pouvant être mis en veille (llama.cpp : `http://<hôte>/health`). S'il ne répond pas, le LLM est réveillé et la question attend son réveil |
| `LLM_WAKE_TIMEOUT_SECONDS`  | `300`                                                     | Durée max du réveil (et de l'attente d'une question)                                                                                                                 |
| `ADMIN_PASSWORD`            | —                                                         | Mot de passe interface admin (sans valeur, l’admin est désactivé)                                                                                                    |
| `ASK_RATE_LIMIT_PER_MINUTE` | `10`                                                      | Questions max par minute et par IP sur `/api/ask` et `/api/ask/stream` (`0` = illimité). Au-delà : HTTP 429 avec `Retry-After`                                       |
| `UPLOADS_DIR`               | `../uploads`                                              | Répertoire des fichiers uploadés                                                                                                                                     |
| `MODEL_PATH`                | `../models/multilingual-e5-small`                         | Chemin du modèle ONNX                                                                                                                                                |
| `MODEL_QUANTIZED`           | `true`                                                    | Télécharger le modèle int8 (118 Mo) au lieu du fp32 (470 Mo)                                                                                                         |
| `ONNX_THREADS`              | `2`                                                       | Threads ONNX Runtime par inférence                                                                                                                                   |

**LLM en veille** : avec `LLM_HEALTH_URL`, la santé du LLM est vérifiée avant chaque question (au plus toutes les 30 s). S'il dort, il est réveillé et la question attend son réveil (`LLM_WAKE_TIMEOUT_SECONDS` max) ; si l'appel échoue juste après un contrôle positif (serveur rendormi, modèle en rechargement), le réveil et l'appel sont relancés une fois. Le frontend appelle `POST /api/llm/warmup` au chargement de la page, au retour sur l'onglet et quand l'utilisateur commence à écrire sa question (au plus toutes les 30 s). Sans `LLM_BASE_URL`, les réponses sont construites uniquement depuis le contexte récupéré (pas de génération).

**Note** : En `ENV=development`, le client LLM ignore les erreurs TLS (InsecureSkipVerify).

---

## Docker

```bash
# Build (télécharge le modèle ONNX automatiquement)
docker build -t ask-rules .

# Lancement
docker run -p 8080:8080 \
  -e DATABASE_URL=postgres://user:pass@host:5432/db \
  -e LLM_BASE_URL=https://api.mistral.ai/v1 \
  -e LLM_API_KEY=your-key \
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
      LLM_BASE_URL: ${LLM_BASE_URL}
      LLM_API_KEY: ${LLM_API_KEY}
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

### Chi Router

- Middleware : Logger, Recoverer, RequestID, RealIP, CORS
- **Timeouts par route** :
  - POST `/api/ask`, `/api/ask/stream` : 60s
  - POST `/api/import` : **pas de timeout** (SSE stream)
  - GET `/api/games` : 30s
  - Admin routes : 10s
  - File serving : 120s

### SSE (Server-Sent Events)

- Import avec **heartbeat 10s** : évite les timeouts proxy/navigateur sur longues importations
- Thread-safe : `sync.Mutex` sur les envois concurrents
- Context cancellation : arrêt propre du heartbeat
- Frontend : ignore les événements `ping`

### Logging

- Format : `[ERROR]` / `[WARN]` avec contexte (route, opération, nom du jeu)
- **Tous les handlers** : logs à chaque erreur HTTP
- **`ask` et `index`** : logs des erreurs métier (jeu introuvable, échec du LLM, etc.)
- Aide au debugging : nom du jeu, opération, erreur originale

### PWA & Service Worker

- Cache-first strategy pour assets statiques
- **Exclusions** :
  - Routes `/api/*` → bypass service worker
  - Header `Accept: text/event-stream` → bypass (SSE)
- Version : `reglomatic-v2`

### Migration automatique

- Exécutée au démarrage (`store.Migrate()`)
- Idempotente : `CREATE TABLE IF NOT EXISTS`, `CREATE INDEX IF NOT EXISTS`
- Extension : `pgvector` ; plein texte natif PostgreSQL (`tsvector`, configuration `french`)

---

## API

### Publique

| Méthode | Route                      | Description                      |
| ------- | -------------------------- | -------------------------------- |
| `POST`  | `/api/ask`                 | Poser une question sur un jeu    |
| `POST`  | `/api/ask/stream`          | Idem, réponse en streaming (SSE) |
| `GET`   | `/api/games`               | Lister les jeux                  |
| `GET`   | `/files/{slug}/{filename}` | Servir un fichier uploadé        |

**Body `/api/ask`** :

```json
{
  "question": "Et à deux joueurs ?",
  "game": "Catan",
  "history": [{ "question": "Comment gagner ?", "answer": "Le premier à 10 points… (p. 4)" }]
}
```

`/api/ask/stream` prend le même body et envoie des Server-Sent Events : `delta` (`{"text": "…"}`, fragment de réponse), puis `done` (même JSON que `/api/ask`) ou `error` (`{"error": "…"}`). Une erreur survenue avant le premier fragment (jeu manquant ou inconnu, limite atteinte…) est renvoyée en JSON avec son code HTTP.

`history` (optionnel) : échanges précédents de la conversation, du plus ancien au plus récent. Le serveur n'en garde que les 3 derniers (questions tronquées à 500 caractères, réponses à 2000), les rejoue au LLM et ajoute la question précédente à la recherche pour comprendre les questions de suivi. Une question avec historique ne passe pas par le cache. Côté interface, la conversation en cours est gardée en mémoire (perdue au rechargement de la page).

### Admin (cookie `admin_session` requis)

| Méthode  | Route                                | Description                                                       |
| -------- | ------------------------------------ | ----------------------------------------------------------------- |
| `POST`   | `/api/admin/login`                   | Connexion                                                         |
| `POST`   | `/api/admin/logout`                  | Déconnexion                                                       |
| `GET`    | `/api/admin/check`                   | Vérifier la session                                               |
| `GET`    | `/api/admin/games`                   | Lister les jeux                                                   |
| `POST`   | `/api/admin/games`                   | Créer / mettre à jour un jeu                                      |
| `DELETE` | `/api/admin/games/{id}`              | Supprimer un jeu                                                  |
| `GET`    | `/api/admin/files`                   | Lister les fichiers uploadés                                      |
| `DELETE` | `/api/admin/files/{slug}/{filename}` | Supprimer un fichier                                              |
| `GET`    | `/api/admin/logs`                    | Logs récents (`?limit=N`)                                         |
| `POST`   | `/api/import`                        | Importer un fichier (SSE) — page `/import`, après connexion admin |
| `POST`   | `/api/admin/games/{id}/reprocess`    | Retraiter un jeu (SSE)                                            |
| `POST`   | `/api/admin/reprocess-all`           | Retraiter tous les jeux (SSE)                                     |
| `GET`    | `/api/admin/jobs/{id}`               | Statut d'un retraitement en cours ou terminé                      |
| `GET`    | `/api/admin/jobs/{id}/stream`        | Se reconnecter au flux SSE d'un retraitement                      |

---

## Embeddings

Le paquet `internal/embed` (`onnx.go`) charge le modèle **multilingual-e5-small** directement via `onnxruntime_go` (CGO). Aucun serveur Python requis.

- **Dimensions** : 384, jusqu'à 512 tokens
- **Langue** : multilingue (optimal pour le français)
- **Modèle quantifié int8** utilisé en priorité s'il est présent (`onnx/model_quantized.onnx`)
- **Préfixes E5** : `query: ` pour les questions (`Embed`), `passage: ` pour les sections indexées (`EmbedPassage`)
- **Session ONNX** : shapes dynamiques (tensors à la taille réelle du texte), arène mémoire désactivée, libérée après 5 min d'inactivité

Si `libonnxruntime.so` ou le modèle est absent au démarrage, le serveur continue sans embeddings (recherche full-text uniquement).

---

## Recherche hybride

`internal/search/search.go` combine :

1. **Recherche vectorielle** — cosinus via `pgvector` (index HNSW)
2. **Full-text search** — `websearch_to_tsquery('french', 'terme1 or terme2 ...')` sur `tsvector` pondéré (titre A, hierarchy_path B, contenu C)
3. **Fusion RRF** (Reciprocal Rank Fusion) — reclassement des deux listes par rang uniquement avec `k=60`

### Évaluer la qualité du retrieval

`server/cmd/eval` mesure Hit@k, MRR et la justesse des numéros de page sur un jeu de questions annotées (`server/eval/questions.json`).
Chaque question liste des extraits du livret attendus dans les sections renvoyées.

```bash
cd server
# Base dédiée : -index supprime et recrée les sections des jeux du dataset
DATABASE_URL=postgresql://postgres:postgres@localhost:5433/askrules_eval go run ./cmd/eval -index -v
```

Relancer l'évaluation avant/après toute modification du chunking, du modèle ou du retriever.

Les requêtes SQL sont dans `internal/store/sections.go`, la fusion dans `internal/search`.

---

## Serveur MCP

`server/cmd/mcp` donne à un assistant (Claude Code…) un accès à la base et au LLM configurés dans `.env`, via MCP en stdio. Il est déclaré dans `.mcp.json` : Claude Code le propose à l'ouverture du projet.

| Outil             | Rôle                                                                                                          |
| ----------------- | ------------------------------------------------------------------------------------------------------------- |
| `db_schema`       | Tables du schéma `public`, colonnes et nombre approximatif de lignes                                          |
| `db_query`        | Une instruction SQL dans une transaction en lecture seule (15 s max, 100 lignes par défaut, 1000 max)         |
| `llm_chat`        | Messages bruts envoyés au LLM (`chat/completions`), sans le prompt de l'arbitre                               |
| `llm_answer_rule` | Question + extraits du livret, avec le prompt de `/api/ask` (sans recherche ni cache) : pour tester le prompt |

Les migrations ne sont jamais lancées. Le LLM en veille est réveillé comme pour `/api/ask` (`LLM_HEALTH_URL`).
Pour une base de production, préférez un rôle PostgreSQL en lecture seule, passé dans `MCP_DATABASE_URL` (prioritaire sur `DATABASE_URL`) :

```sql
CREATE ROLE askrules_ro LOGIN PASSWORD '…';
GRANT CONNECT ON DATABASE railway TO askrules_ro;
GRANT USAGE ON SCHEMA public TO askrules_ro;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO askrules_ro;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO askrules_ro;
```

Lancement manuel (depuis `server/`) : `go run ./cmd/mcp`. Ailleurs que Claude Code (Claude Desktop…), déclarez la commande `go -C <chemin>/server run ./cmd/mcp`.

---

## Interface d'administration

Accessible sur `/admin` (mot de passe via `ADMIN_PASSWORD`) :

- **`/admin/games`** — liste des jeux avec nombre de sections, retraitement individuel ou global (suivi SSE temps réel)
- **`/admin/files`** — fichiers uploadés groupés par jeu, suppression
- **`/admin/logs`** — journal des événements groupé par date
- **`/import`** — import de nouveaux fichiers avec progression live (SSE)

## Choix du modèle

Modèle actuel : **Gemma 3 1B** (avant : un Qwen d'environ 0,8B). Le prompt (`server/internal/llm/client.go`) est écrit pour un modèle de cette taille : consignes courtes et numérotées, exemples de format, un seul message utilisateur (rejouer les échanges précédents faisait répondre « Oui » tout court au bout de quelques questions).

À cette taille, attendez-vous à un français parfois maladroit (voire des réponses en anglais), à des consignes suivies imparfaitement, et à des règles ou des pages inventées. Si c'est fréquent, envoyer 4 sections au lieu de 6 raccourcit le contexte.

Meilleurs modèles si le matériel le permet : Qwen3-4B-Instruct-2507 (sans mode « thinking », donc rapide), Gemma 3 4B (français souvent plus naturel) ou Ministral 3B/8B. Pour comparer, posez une dizaine de questions de `server/eval/questions.json` et vérifiez le français, les pages citées et l'absence de règles inventées.
