#!/usr/bin/env bash
# scripts/download-model.sh — Télécharge le modèle ONNX multilingual-e5-small
#
# Usage :
#   ./scripts/download-model.sh                     # → models/multilingual-e5-small/
#   ./scripts/download-model.sh /chemin/custom      # répertoire cible personnalisé
#
# Prérequis : curl, unzip (ou python3 avec huggingface_hub en fallback)

set -euo pipefail

DEST="${1:-models/multilingual-e5-small}"
REPO="intfloat/multilingual-e5-small"
BASE_URL="https://huggingface.co/${REPO}/resolve/main"

# Fichiers nécessaires pour l'inférence ONNX
FILES=(
  "config.json"
  "tokenizer.json"
  "tokenizer_config.json"
  "special_tokens_map.json"
  "sentencepiece.bpe.model"
  "onnx/model.onnx"
)

echo "📥 Téléchargement de ${REPO}"
echo "   Destination : ${DEST}"
echo ""

mkdir -p "${DEST}/onnx"

for FILE in "${FILES[@]}"; do
  URL="${BASE_URL}/${FILE}"
  OUT="${DEST}/${FILE}"
  # Créer le sous-dossier si nécessaire
  mkdir -p "$(dirname "${OUT}")"

  if [[ -f "${OUT}" ]]; then
    echo "  ✔ (déjà présent) ${FILE}"
    continue
  fi

  echo -n "  ↓ ${FILE} ... "
  if curl --retry 3 --retry-delay 2 -o "${OUT}" "${URL}"; then
    SIZE=$(du -sh "${OUT}" | cut -f1)
    echo "${SIZE}"
  else
    echo "ERREUR"
    rm -f "${OUT}"
    # Fallback Python si curl échoue (token HuggingFace requis pour les dépôts privés)
    if command -v python3 &>/dev/null; then
      echo "    → Tentative via huggingface_hub Python..."
      python3 -c "
from huggingface_hub import hf_hub_download
hf_hub_download(repo_id='${REPO}', filename='${FILE}', local_dir='${DEST}')
print('    ✔ OK')
" || { echo "    ✖ Échec. Vérifiez votre connexion ou installez : pip install huggingface_hub"; exit 1; }
    else
      echo "    ✖ curl a échoué et python3 n'est pas disponible."
      exit 1
    fi
  fi
done

echo ""
echo "✅ Modèle prêt dans : ${DEST}"
echo ""
echo "Contenu :"
find "${DEST}" -type f | sort | while read -r f; do
  SIZE=$(du -sh "$f" | cut -f1)
  echo "  ${SIZE}  ${f#${DEST}/}"
done
