#!/usr/bin/env bash
# Creates .env for you: asks for the Gemini key (hidden), picks a model, and
# generates a random database password. Run from the ielts-app folder:
#   bash scripts/init-env.sh          (refuses to overwrite an existing .env)
#   bash scripts/init-env.sh --force  (overwrites it)
set -euo pipefail
cd "$(dirname "$0")/.."

if [[ -f .env && "${1:-}" != "--force" ]]; then
  echo ".env already exists. Edit it directly, or run with --force to replace it." >&2
  exit 1
fi

read -rsp "Paste your Gemini API key (input is hidden): " KEY; echo
if [[ -z "$KEY" ]]; then echo "No key entered." >&2; exit 1; fi
read -rp "Model name [gemini-3.5-flash-lite]: " MODEL
MODEL="${MODEL:-gemini-3.5-flash-lite}"

# Letters and digits only, because the password is placed inside a URL.
if command -v openssl >/dev/null 2>&1; then
  PASS="$(openssl rand -hex 16)"
else
  PASS="$(LC_ALL=C tr -dc 'a-f0-9' </dev/urandom | head -c 32)"
fi

umask 177   # the file is created readable by you only
cat > .env <<EOT
GEMINI_API_KEY=$KEY
GEMINI_MODEL=$MODEL
POSTGRES_USER=ielts
POSTGRES_DB=ielts
POSTGRES_PASSWORD=$PASS
WEB_PORT=8080
API_PORT=8081
EVAL_RATE_PER_MINUTE=10
NEW_VOCAB_PER_DAY=0
EOT
unset KEY PASS
echo ".env created (permissions 600). Next: docker compose up -d --build"
