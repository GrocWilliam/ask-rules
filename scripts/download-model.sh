#!/usr/bin/env bash
# scripts/download-model.sh — Télécharge le modèle ONNX multilingual-e5-small
#
# Usage :
#   ./scripts/download-model.sh                     # → models/multilingual-e5-small/
#   ./scripts/download-model.sh /chemin/custom      # répertoire cible personnalisé
#   MODEL_QUANTIZED=false ./scripts/download-model.sh  # modèle fp32 (470 Mo au lieu de 118 Mo)
#
# Par défaut, le modèle quantifié int8 (export Xenova du même modèle) est utilisé :
# ~4x moins de RAM pour une perte de qualité négligeable.
#
# Prérequis : curl, unzip (ou python3 avec huggingface_hub en fallback)

set -euo pipefail

DEST="${1:-models/multilingual-e5-small}"
REPO="intfloat/multilingual-e5-small"

# Fichiers nécessaires pour l'inférence ONNX
FILES=(
  "config.json"
  "tokenizer.json"
  "tokenizer_config.json"
  "special_tokens_map.json"
  "sentencepiece.bpe.model"
)

QUANTIZED_REPO="Xenova/multilingual-e5-small"
if [[ "${MODEL_QUANTIZED:-true}" == "true" ]]; then
  # Préfixe "repo|" : fichier pris dans un autre dépôt que ${REPO}
  FILES+=("${QUANTIZED_REPO}|onnx/model_quantized.onnx")
else
  FILES+=("onnx/model.onnx")
fi

echo "📥 Téléchargement de ${REPO}"
echo "   Destination : ${DEST}"
echo ""

mkdir -p "${DEST}/onnx"

for ENTRY in "${FILES[@]}"; do
  if [[ "${ENTRY}" == *"|"* ]]; then
    FILE_REPO="${ENTRY%%|*}"
    FILE="${ENTRY#*|}"
  else
    FILE_REPO="${REPO}"
    FILE="${ENTRY}"
  fi
  URL="https://huggingface.co/${FILE_REPO}/resolve/main/${FILE}"
  OUT="${DEST}/${FILE}"
  # Créer le sous-dossier si nécessaire
  mkdir -p "$(dirname "${OUT}")"

  if [[ -f "${OUT}" ]]; then
    # Vérifier que les fichiers .json existants sont valides (protection contre cache corrompu)
    if [[ "${OUT}" == *.json ]]; then
      FIRST_CHAR=$(head -c1 "${OUT}" 2>/dev/null)
      if [[ "${FIRST_CHAR}" == "{" || "${FIRST_CHAR}" == "[" ]]; then
        echo "  ✔ (déjà présent) ${FILE}"
        continue
      else
        echo "  ✖ (corrompu, re-téléchargement) ${FILE}"
        rm -f "${OUT}"
      fi
    else
      echo "  ✔ (déjà présent) ${FILE}"
      continue
    fi
  fi

  echo -n "  ↓ ${FILE} ... "
  if curl -fsSL --retry 3 --retry-delay 2 -o "${OUT}" "${URL}"; then
    SIZE=$(du -sh "${OUT}" | cut -f1)
    # Vérifier que les fichiers .json contiennent du JSON valide
    if [[ "${OUT}" == *.json ]]; then
      FIRST_CHAR=$(head -c1 "${OUT}")
      if [[ "${FIRST_CHAR}" != "{" && "${FIRST_CHAR}" != "[" ]]; then
        echo "INVALIDE (contenu non-JSON, premier caractère: '${FIRST_CHAR}')"
        echo "    → Le fichier est probablement une réponse d'erreur HTTP (redirection manquée ?)"
        rm -f "${OUT}"
        exit 1
      fi
    fi
    echo "${SIZE}"
  else
    echo "ERREUR"
    rm -f "${OUT}"
    # Fallback Python si curl échoue (token HuggingFace requis pour les dépôts privés)
    if command -v python3 &>/dev/null; then
      echo "    → Tentative via huggingface_hub Python..."
      python3 -c "
from huggingface_hub import hf_hub_download
hf_hub_download(repo_id='${FILE_REPO}', filename='${FILE}', local_dir='${DEST}')
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
