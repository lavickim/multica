#!/usr/bin/env bash
set -euo pipefail

PUBLIC_PORT="${PORT:-8080}"
BACKEND_PORT="${MULTICA_BACKEND_PORT:-8081}"
FRONTEND_PORT="${MULTICA_FRONTEND_PORT:-3000}"
FRONTEND_HOST="${MULTICA_FRONTEND_HOST:-0.0.0.0}"

/app/migrate up

PORT="${BACKEND_PORT}" /app/server &
backend_pid=$!

PORT="${FRONTEND_PORT}" HOSTNAME="${FRONTEND_HOST}" node /app/web/apps/web/server.js &
frontend_pid=$!

PUBLIC_PORT="${PUBLIC_PORT}" caddy run --config /etc/caddy/Caddyfile --adapter caddyfile &
caddy_pid=$!

cleanup() {
  kill "${backend_pid}" "${frontend_pid}" "${caddy_pid}" 2>/dev/null || true
}

trap cleanup EXIT INT TERM

wait -n "${backend_pid}" "${frontend_pid}" "${caddy_pid}"
status=$?

cleanup
wait || true

exit "${status}"
