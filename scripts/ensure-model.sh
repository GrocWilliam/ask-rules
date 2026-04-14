#!/usr/bin/env sh
set -eu

echo "[boot] Starting standalone backend container"

MODEL_DIR="${MODEL_PATH:-/app/models/multilingual-e5-small}"
MODEL_FILE="$MODEL_DIR/onnx/model.onnx"
TOKENIZER_FILE="$MODEL_DIR/tokenizer.json"
AUTO_DOWNLOAD="${MODEL_AUTO_DOWNLOAD:-true}"

if [ -f "$MODEL_FILE" ] && [ -f "$TOKENIZER_FILE" ]; then
  echo "[model] ONNX model already present in $MODEL_DIR"
else
  echo "[model] ONNX model missing in $MODEL_DIR"

  if [ "$AUTO_DOWNLOAD" = "true" ]; then
    echo "[model] Downloading model files for deployment..."
    mkdir -p "$MODEL_DIR"
    /app/scripts/download-model.sh "$MODEL_DIR"
  else
    echo "[model] Auto-download disabled. Mount the models directory or enable MODEL_AUTO_DOWNLOAD=true."
    exit 1
  fi
fi

exec /app/ask-rules-server
