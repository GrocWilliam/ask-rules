# syntax=docker/dockerfile:1
# ════════════════════════════════════════════════════════════════════════════
# ask-rules — Image de production
# Architecture : SvelteKit (adapter-static, embarqué) + binaire Go + onnxruntime
#
# Construction : docker build -t ask-rules .
#
# Lancement :
#   docker run -p 8080:8080 \
#     -e DATABASE_URL=postgresql://.... \
#     -e MISTRAL_API_KEY=.... \
#     ask-rules
# 
# Le modèle ONNX est intégré dans l'image (téléchargé au docker build).
# ════════════════════════════════════════════════════════════════════════════

ARG ONNX_VERSION=1.20.0

# ── Stage 1 : Build SvelteKit (adapter-static → server/build/) ──────────────
FROM node:24-slim AS web-builder
WORKDIR /workspace

RUN npm install -g pnpm@10 --no-update-notifier --quiet
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile

COPY svelte.config.js vite.config.mts tsconfig.json tsconfig.node.json ./
COPY src ./src
COPY static ./static

RUN pnpm run build:web
# Résultat dans server/build/ (lu par go:embed dans server/cmd/server/main.go)

# ── Stage 2 : Build Go (CGO + onnxruntime) ───────────────────────────────────
FROM golang:1.22-bookworm AS go-builder
ARG ONNX_VERSION
WORKDIR /workspace

# Outils C requis par CGO (onnxruntime_go)
RUN apt-get update && apt-get install -y --no-install-recommends \
    gcc g++ ca-certificates curl \
    && rm -rf /var/lib/apt/lists/*

# Téléchargement de libonnxruntime.so (version correspondant à yalue/onnxruntime_go)
RUN curl -fsSL \
    "https://github.com/microsoft/onnxruntime/releases/download/v${ONNX_VERSION}/onnxruntime-linux-x64-${ONNX_VERSION}.tgz" \
    -o /tmp/ort.tgz \
    && tar -xzf /tmp/ort.tgz -C /tmp \
    && cp /tmp/onnxruntime-linux-x64-${ONNX_VERSION}/lib/libonnxruntime.so.${ONNX_VERSION} \
          /usr/local/lib/libonnxruntime.so \
    && ldconfig \
    && rm -rf /tmp/ort*

# Cache des dépendances Go (layer dédié, invalidé seulement si go.mod change)
COPY server/go.mod server/go.sum ./server/
RUN cd server && go mod download

# Code source Go + artefacts SvelteKit (go:embed server/build)
COPY server ./server
COPY --from=web-builder /workspace/server/build ./server/build

# Téléchargement du modèle ONNX (intégré dans l'image, pas de volume requis)
COPY scripts/download-model.sh ./scripts/download-model.sh
RUN apt-get install -y --no-install-recommends bash \
    && rm -rf /var/lib/apt/lists/* \
    && bash scripts/download-model.sh models/multilingual-e5-small

# Compilation du binaire (stripped pour réduire la taille)
RUN cd server && CGO_ENABLED=1 go build -ldflags="-w -s" -o /ask-rules-server ./cmd/server

# ── Stage 3 : Image de production minimale ───────────────────────────────────
FROM debian:bookworm-slim AS runtime
ARG ONNX_VERSION
WORKDIR /app

# Certificats SSL (requêtes HTTPS vers LLM APIs) + poppler-utils (pdftotext)
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    poppler-utils \
    && rm -rf /var/lib/apt/lists/*

# Bibliothèque onnxruntime partagée (runtime CGO)
COPY --from=go-builder /usr/local/lib/libonnxruntime.so /usr/local/lib/libonnxruntime.so
RUN ldconfig

# Binaire Go (embarque déjà server/build/ via go:embed — pas besoin de Node.js)
COPY --from=go-builder /ask-rules-server ./ask-rules-server

# Modèle ONNX téléchargé au build (intégré dans l'image)
COPY --from=go-builder /workspace/models ./models

# uploads/ créé au démarrage si besoin (chemin configuré via UPLOADS_DIR)
RUN mkdir -p uploads

# ── Variables d'environnement ─────────────────────────────────────────────────
# Obligatoires :
#   DATABASE_URL     — ex: postgres://user:pass@host:5432/db
# Optionnelles :
#   MISTRAL_API_KEY  — LLM Mistral
#   OPENAI_API_KEY   — LLM OpenAI
#   OPENAI_MODEL     — défaut: gpt-4o-mini
#   OLLAMA_HOST      — ex: http://ollama:11434
#   OLLAMA_MODEL     — ex: llama3
#   ADMIN_PASSWORD   — mot de passe admin (défaut: admin)
#   REDIS_ENABLED    — true/false (défaut: false)
#   REDIS_URL        — ex: redis://redis:6379
ENV PORT=8080 \
    MODEL_PATH=/app/models/multilingual-e5-small \
    UPLOADS_DIR=/app/uploads \
    LD_LIBRARY_PATH=/usr/local/lib

EXPOSE 8080

CMD ["/app/ask-rules-server"]
