#!/usr/bin/env bash
# Expands vocabulary from a fixed word list, in chunks, with a strong model.
# This is manual and resumable: words already in the database are skipped.
# Run from the ielts-app folder:
#
#   bash scripts/expand-vocab.sh
#   bash scripts/expand-vocab.sh content/words/anki-b2.txt
#
# Environment variables:
#   MODEL    Gemini model (default: gemini-3.5-flash)
#   THINKING thinking budget (default: 0, off)
#   CHUNK    words per model call, 1 to 30 (default: 20)
#   RETRIES  retries for skipped words, 0 to 5 (default: 2)
#   SKIP_BUILD=1  use the current api image without rebuilding it
set -euo pipefail
cd "$(dirname "$0")/.."

usage() {
  cat <<'EOT'
Usage: bash scripts/expand-vocab.sh [words-file]

  words-file  tab-separated word, tier, CEFR (default: content/words/anki-b2.txt)
              The file must be under content/ so the api container can read it.

Environment:
  MODEL    Gemini model (default: gemini-3.5-flash)
  THINKING thinking budget (default: 0, off; -1 uses the model default)
  CHUNK    words per model call, 1 to 30 (default: 20)
  RETRIES  retries for skipped words, 0 to 5 (default: 2)
  SKIP_BUILD=1  do not rebuild the api image first

The command makes real Gemini calls. It validates every item, drops duplicates
and invalid output, writes batch files under content/batches/, and inserts the
accepted items into the database. Generated material is AI-written, not
official IELTS material. Review samples before relying on it.
EOT
}

WORDS="${1:-content/words/anki-b2.txt}"
MODEL="${MODEL:-gemini-3.5-flash}"
THINKING="${THINKING:-0}"
CHUNK="${CHUNK:-20}"
RETRIES="${RETRIES:-2}"

case "$WORDS" in
  -h|--help)
    usage
    exit 0
    ;;
esac

if [[ ! -f "$WORDS" ]]; then
  echo "expand-vocab: word list not found: $WORDS" >&2
  exit 1
fi
case "$WORDS" in
  content/*) ;;
  *)
    echo "expand-vocab: the word list must be under content/ so the container can read it." >&2
    exit 2
    ;;
esac
if [[ ! -f .env ]]; then
  echo "expand-vocab: .env is missing. Create it with: bash scripts/init-env.sh" >&2
  exit 1
fi
if ! grep -Eq '^GEMINI_API_KEY=.+' .env; then
  echo "expand-vocab: GEMINI_API_KEY is not set in .env." >&2
  exit 1
fi
if ! command -v docker >/dev/null 2>&1; then
  echo "expand-vocab: docker is not installed or not on PATH." >&2
  exit 1
fi
if ! docker compose version >/dev/null 2>&1; then
  echo "expand-vocab: docker compose is not available." >&2
  exit 1
fi

TOTAL="$(grep -cve '^[[:space:]]*$' "$WORDS" || true)"
CONTAINER_PATH="/content/${WORDS#content/}"

echo "Word list: $WORDS ($TOTAL entries)"
echo "Model:     $MODEL (thinking budget $THINKING)"
echo "Chunk:     $CHUNK words per call, $RETRIES retries for skipped words"
echo

if [[ "${SKIP_BUILD:-0}" != "1" ]]; then
  echo "Building the api image so the generator includes the latest code."
  docker compose build api
  echo
fi

docker compose run --rm --user root api generate \
  -type vocab \
  -words "$CONTAINER_PATH" \
  -chunk "$CHUNK" \
  -retries "$RETRIES" \
  -model "$MODEL" \
  -thinking "$THINKING"

echo
echo "Accepted items were inserted into the database and the batch files are in content/batches/."
echo "The app can use them immediately; no restart is needed."
echo "Generated material is AI-written. Check it before relying on it; it is not official IELTS material."
