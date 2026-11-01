#!/usr/bin/env bash
# Generates new vocabulary or writing material once, on demand. Nothing runs on
# a schedule. Run from the ielts-app folder:
#
#   bash scripts/generate-content.sh vocab 10
#   bash scripts/generate-content.sh writing 5
#
# The count is optional: vocab defaults to 10 and writing to 5.
# Environment: MODEL (default gemini-3.5-flash), THINKING (default 0, off).
set -euo pipefail
cd "$(dirname "$0")/.."

usage() {
  cat <<'EOT'
Usage: bash scripts/generate-content.sh <vocab|writing> [count]

  count  how many new items to keep, from 1 to 30
         (default: vocab 10, writing 5)

The generator calls Gemini, validates the output, drops duplicates, writes a
batch file under content/batches/, and inserts the accepted items into the
database. Review the output: it is AI-written, not official IELTS material.
EOT
}

TYPE="${1:-}"
COUNT="${2:-}"
MODEL="${MODEL:-gemini-3.5-flash}"
THINKING="${THINKING:-0}"

case "$TYPE" in
  -h|--help)
    usage
    exit 0
    ;;
  "")
    usage >&2
    exit 2
    ;;
esac

case "$TYPE" in
  vocab)
    COUNT="${COUNT:-10}"
    ;;
  writing)
    COUNT="${COUNT:-5}"
    ;;
  *)
    echo "generate-content: type must be vocab or writing." >&2
    usage >&2
    exit 2
    ;;
esac

if ! [[ "$COUNT" =~ ^[0-9]+$ ]] || (( COUNT < 1 || COUNT > 30 )); then
  echo "generate-content: count must be a whole number from 1 to 30." >&2
  exit 2
fi

if [[ ! -f .env ]]; then
  echo "generate-content: .env is missing. Create it with: bash scripts/init-env.sh" >&2
  exit 1
fi
if ! grep -Eq '^GEMINI_API_KEY=.+' .env; then
  echo "generate-content: GEMINI_API_KEY is not set in .env." >&2
  exit 1
fi
if ! command -v docker >/dev/null 2>&1; then
  echo "generate-content: docker is not installed or not on PATH." >&2
  exit 1
fi
if ! docker compose version >/dev/null 2>&1; then
  echo "generate-content: docker compose is not available." >&2
  exit 1
fi

echo "Generating $COUNT $TYPE item(s) with $MODEL. This makes real Gemini calls and can take a minute."
docker compose build api
docker compose run --rm --user root api generate -type "$TYPE" -count "$COUNT" -model "$MODEL" -thinking "$THINKING"

echo
echo "Accepted items were inserted into the database and the batch file is in content/batches/."
echo "The app can use them immediately; no restart is needed."
echo "Generated material is AI-written. Check it before relying on it; it is not official IELTS material."
