#!/usr/bin/env bash
# Quick check of a running stack. NOTE: step 3 sends one real answer to the AI,
# so it uses one request of your daily quota.
# Run from the ielts-app folder after "docker compose up": ./scripts/smoke.sh
set -euo pipefail
BASE="${BASE:-http://127.0.0.1:${WEB_PORT:-8080}}"
pretty() { if command -v jq >/dev/null; then jq "$@"; else cat; fi; }

echo "1. health"
curl -fsS "$BASE/api/health" | pretty .

echo; echo "2. next exercise"
EX="$(curl -fsS "$BASE/api/exercises/next?type=vocab")"
echo "$EX" | pretty .
ID="$(echo "$EX" | python3 -c 'import sys,json;print(json.load(sys.stdin)["id"])')"

echo; echo "3. submit an answer (real AI call)"
UUID="$(python3 -c 'import uuid;print(uuid.uuid4())')"
curl -sS -w '\n[HTTP %{http_code}]\n' "$BASE/api/attempts" \
  -d "{\"id\":\"$UUID\",\"exercise_id\":$ID,\"answer\":\"It means to give something to a group for a purpose.\"}" | pretty .

echo; echo "4. statistics"
curl -fsS "$BASE/api/stats?tz=UTC" | pretty '{totals, streak_days, top_categories}'
