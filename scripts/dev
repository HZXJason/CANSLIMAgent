#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

BACKEND_HEALTH="${BACKEND_HEALTH:-http://localhost:8080/api/v1/health}"
SIDECAR_HEALTH="${SIDECAR_HEALTH:-http://127.0.0.1:8765/health}"
FRONTEND_URL="${FRONTEND_URL:-http://127.0.0.1:5173/}"
STARTED_PIDS=()

log() {
  printf '\n[%s] %s\n' "$(date '+%H:%M:%S')" "$*"
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'Missing required command: %s\n' "$1" >&2
    exit 1
  fi
}

url_ok() {
  curl -fsS "$1" >/dev/null 2>&1
}

wait_url() {
  local name="$1"
  local url="$2"
  local attempts="${3:-60}"

  for ((i = 1; i <= attempts; i++)); do
    if url_ok "$url"; then
      log "$name is ready: $url"
      return 0
    fi
    sleep 1
  done

  printf '%s did not become ready: %s\n' "$name" "$url" >&2
  return 1
}

wait_postgres() {
  for ((i = 1; i <= 60; i++)); do
    local pg_health
    pg_health="$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' alphalens-postgres 2>/dev/null || true)"
    if [ "$pg_health" = "healthy" ] || [ "$pg_health" = "running" ]; then
      log "Postgres is ready: alphalens-postgres ($pg_health)"
      return 0
    fi
    sleep 1
  done

  docker logs --tail=80 alphalens-postgres 2>/dev/null || true
  printf 'Postgres did not become ready.\n' >&2
  return 1
}

cleanup() {
  trap - EXIT INT TERM
  if [ "${#STARTED_PIDS[@]}" -gt 0 ]; then
    log "Stopping local services started by this script"
    for pid in "${STARTED_PIDS[@]}"; do
      kill "$pid" >/dev/null 2>&1 || true
    done
    wait >/dev/null 2>&1 || true
  fi

  if [ "${STOP_DOCKER_ON_EXIT:-0}" = "1" ]; then
    log "Stopping Docker postgres"
    docker compose stop postgres >/dev/null
  else
    log "Docker postgres is still running. Set STOP_DOCKER_ON_EXIT=1 to stop it on exit."
  fi
}

on_signal() {
  cleanup
  exit 130
}

trap cleanup EXIT
trap on_signal INT TERM

require_cmd docker
require_cmd curl
require_cmd go
require_cmd npm

log "Starting Docker postgres"
docker compose up -d postgres
wait_postgres

if url_ok "$SIDECAR_HEALTH"; then
  log "Sidecar already running: $SIDECAR_HEALTH"
else
  log "Starting Python sidecar"
  (cd sidecar && ./start.sh) &
  STARTED_PIDS+=("$!")
  wait_url "Sidecar" "$SIDECAR_HEALTH" 90
fi

if url_ok "$BACKEND_HEALTH"; then
  log "Backend already running: $BACKEND_HEALTH"
else
  log "Starting Go backend"
  (cd backend && go run ./cmd/server) &
  STARTED_PIDS+=("$!")
  wait_url "Backend" "$BACKEND_HEALTH" 60
fi

if url_ok "$FRONTEND_URL"; then
  log "Frontend already running: $FRONTEND_URL"
else
  log "Starting React frontend"
  (
    cd frontend
    if [ ! -d node_modules ]; then
      npm install
    fi
    npm run dev -- --host 127.0.0.1
  ) &
  STARTED_PIDS+=("$!")
  wait_url "Frontend" "$FRONTEND_URL" 60
fi

cat <<EOF

AlphaLens is ready:
  Frontend: $FRONTEND_URL
  Backend:  $BACKEND_HEALTH
  Sidecar:  $SIDECAR_HEALTH
  Database: Docker service postgres / container alphalens-postgres

Press Ctrl+C to stop services started by this script.
EOF

if [ "${#STARTED_PIDS[@]}" -eq 0 ]; then
  log "All services were already running."
  trap - EXIT INT TERM
  exit 0
fi

wait
