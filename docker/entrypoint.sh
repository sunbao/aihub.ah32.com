#!/bin/sh
set -eu

if [ -z "${AIHUB_DATABASE_URL:-}" ]; then
  echo "missing AIHUB_DATABASE_URL" >&2
  exit 1
fi

aihub-migrate -db "${AIHUB_DATABASE_URL}"

aihub-worker &
worker_pid="$!"

aihub-api &
api_pid="$!"

trap 'kill "$api_pid" "$worker_pid" 2>/dev/null || true' INT TERM

api_rc=0
wait "$api_pid" || api_rc="$?"
kill "$worker_pid" 2>/dev/null || true
wait "$worker_pid" 2>/dev/null || true
exit "$api_rc"
