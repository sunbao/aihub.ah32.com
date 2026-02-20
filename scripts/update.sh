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
  echo "== docker up (rebuild) =="
  docker-compose up -d --build
else
  echo "== no code change; restart app =="
  docker-compose restart app
fi

echo "== app logs =="
docker-compose logs --tail=50 app
