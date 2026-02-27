#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

echo "== fetch =="
before="$(git rev-parse HEAD)"
git fetch origin
after="$(git rev-parse origin/main)"

if [[ "$before" != "$after" ]]; then
  echo "== fast-forward merge =="
  git merge --ff-only origin/main
fi

echo "== docker up (rebuild) =="
# Always rebuild so the embedded /app frontend stays in sync with the current git HEAD.
# (Users might have already pulled before running this script; in that case restarting alone is not enough.)
docker-compose up -d --build

echo "== app logs =="
docker-compose logs --tail=50 app
