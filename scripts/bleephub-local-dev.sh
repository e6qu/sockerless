#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

APP_NAME="bleephub-server"
SERVER_DIR="$REPO_ROOT/bleephub"
UI_DIR="$REPO_ROOT/ui/packages/bleephub"
LOCAL_ROOT="$REPO_ROOT/.local/bleephub"

API_PORT="${BLEEPHUB_API_PORT:-5555}"
UI_PORT="${BLEEPHUB_UI_PORT:-5173}"
DATA_DIR="${BLEEPHUB_DATA_DIR:-$LOCAL_ROOT/data}"
GIT_DIR="${BLEEPHUB_GIT_DIR:-$LOCAL_ROOT/git}"
ADMIN_TOKEN="${BLEEPHUB_ADMIN_TOKEN:-bleephub-admin-token-00000000000000000000}"
TLS_DIR="$LOCAL_ROOT/tls"
PID_FILE="$LOCAL_ROOT/local-dev.pid"
SERVER_LOG="$LOCAL_ROOT/server.log"
UI_LOG="$LOCAL_ROOT/ui.log"

TLS=0
DEV=0
BUILD=1
YES=0

usage() {
  cat <<EOF
Usage: $0 <command> [options]

Commands:
  start            Build and start bleephub API + UI + storage.
  stop             Stop the running local-dev processes.
  restart          Stop and start again.
  status           Show whether the processes are running.
  logs             Tail the server log (use -f to follow).
  clean            Remove local data, logs, and PID files.

Options:
  --dev            Start the Vite dev server on :$UI_PORT (API still on :$API_PORT).
  --tls            Use HTTPS on :8443 with a self-signed cert in $TLS_DIR.
  --no-build       Skip compiling; use existing bleephub-server binary / UI dist.
  --yes            Skip confirmation for destructive commands.
  -h, --help       Show this help.

Environment overrides:
  BLEEPHUB_API_PORT, BLEEPHUB_UI_PORT, BLEEPHUB_DATA_DIR,
  BLEEPHUB_GIT_DIR, BLEEPHUB_ADMIN_TOKEN,
  BLEEPHUB_OBJECT_S3_BUCKET, BLEEPHUB_OBJECT_S3_ENDPOINT,
  BLEEPHUB_OBJECT_S3_PREFIX

Examples:
  $0 start
  $0 start --dev
  $0 start --tls
  $0 stop
  $0 clean
EOF
}

die() {
  printf 'Error: %s\n' "$1" >&2
  exit 1
}

check_deps() {
  command -v go >/dev/null 2>&1 || die 'go is required'
  command -v make >/dev/null 2>&1 || die 'make is required'
  command -v bun >/dev/null 2>&1 || die 'bun is required'
  command -v curl >/dev/null 2>&1 || die 'curl is required'
}

require_object_storage() {
  if [ -z "${BLEEPHUB_OBJECT_S3_BUCKET:-}" ]; then
    die "BLEEPHUB_OBJECT_S3_BUCKET is required because local-dev starts persisted Bleephub; point it at an S3-compatible bucket such as MinIO for Actions artifacts, dependency caches, runner logs, release assets, package files, container-registry blobs, CodeQL database archives, CodeQL variant-analysis query packs, and artifact attestation bundles"
  fi
}

ensure_dirs() {
  mkdir -p "$DATA_DIR" "$GIT_DIR" "$LOCAL_ROOT"
}

build_ui() {
  printf '▸ Building UI…\n'
  (
    cd "$UI_DIR"
    bun install
    bun run build
  )
}

build_server() {
  printf '▸ Building %s…\n' "$APP_NAME"
  (
    cd "$SERVER_DIR"
    if [ "$DEV" -eq 1 ]; then
      make build-noui
    else
      make build
    fi
  )
}

generate_tls() {
  mkdir -p "$TLS_DIR"
  local cert="$TLS_DIR/bph.crt"
  local key="$TLS_DIR/bph.key"
  if [ ! -f "$cert" ] || ! openssl x509 -checkend 86400 -noout -in "$cert" >/dev/null 2>&1; then
    printf '▸ Generating self-signed TLS cert in %s…\n' "$TLS_DIR"
    command -v openssl >/dev/null 2>&1 || die 'openssl is required for --tls'
    openssl req -x509 -newkey rsa:2048 -days 825 -nodes \
      -keyout "$key" -out "$cert" \
      -subj "/CN=localhost" \
      -addext "subjectAltName=DNS:localhost,IP:127.0.0.1"
  fi
}

read_pids() {
  if [ ! -f "$PID_FILE" ]; then
    return 0
  fi
  while IFS='=' read -r name pid; do
    case "$name" in
      server) SERVER_PID="$pid" ;;
      ui) UI_PID="$pid" ;;
    esac
  done < "$PID_FILE"
}

is_running() {
  [ -n "${1:-}" ] && kill -0 "$1" >/dev/null 2>&1
}

wait_for_health() {
  local url="$1"
  for _ in $(seq 1 30); do
    if curl -fsS "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  return 1
}

start() {
  check_deps
  require_object_storage
  ensure_dirs

  if [ "$TLS" -eq 1 ]; then
    if [ -z "${BLEEPHUB_API_PORT:-}" ]; then
      API_PORT=8443
    fi
    generate_tls
  fi

  if [ "$BUILD" -eq 1 ]; then
    if [ "$DEV" -ne 1 ]; then
      build_ui
    fi
    build_server
  fi

  if is_running "${SERVER_PID:-}"; then
    die "bleephub-server is already running (PID $SERVER_PID). Run '$0 stop' first."
  fi
  if is_running "${UI_PID:-}"; then
    die "Vite dev server is already running (PID $UI_PID). Run '$0 stop' first."
  fi

  rm -f "$SERVER_LOG" "$UI_LOG"

  local scheme="http"
  local external_url="http://localhost:$API_PORT"
  local env_vars=(
    "BLEEPHUB_PERSIST=true"
    "BLEEPHUB_DATA_DIR=$DATA_DIR"
    "BLEEPHUB_GIT_DIR=$GIT_DIR"
    "BLEEPHUB_ADMIN_TOKEN=$ADMIN_TOKEN"
    "BLEEPHUB_EXTERNAL_URL=$external_url"
  )

  if [ "$TLS" -eq 1 ]; then
    scheme="https"
    env_vars+=("BPH_TLS_CERT=$TLS_DIR/bph.crt")
    env_vars+=("BPH_TLS_KEY=$TLS_DIR/bph.key")
  fi

  printf '▸ Starting bleephub API on %s://localhost:%s…\n' "$scheme" "$API_PORT"
  (
    cd "$SERVER_DIR"
    env "${env_vars[@]}" \
      nohup "./$APP_NAME" --addr ":$API_PORT" --log-level info \
      > "$SERVER_LOG" 2>&1 &
    echo "server=$!" > "$PID_FILE"
  )

  if ! wait_for_health "$scheme://localhost:$API_PORT/health"; then
    printf 'Error: bleephub-server did not become healthy. See %s\n' "$SERVER_LOG" >&2
    stop
    exit 1
  fi

  if [ "$DEV" -eq 1 ]; then
    printf '▸ Starting Vite dev server on http://localhost:%s…\n' "$UI_PORT"
    (
      cd "$UI_DIR"
      nohup bun run dev -- --port "$UI_PORT" \
        > "$UI_LOG" 2>&1 &
      echo "ui=$!" >> "$PID_FILE"
    )
    sleep 2
    if ! curl -fsS "http://localhost:$UI_PORT/ui/" >/dev/null 2>&1; then
      printf 'Warning: Vite dev server is not responding yet. See %s\n' "$UI_LOG" >&2
    fi
  fi

  printf '\n'
  printf 'bleephub is running.\n'
  printf '\n'
  printf 'Endpoints:\n'
  printf '  API base      %s://localhost:%s/\n' "$scheme" "$API_PORT"
  printf '  UI            %s://localhost:%s/ui/\n' "$scheme" "$API_PORT"
  printf '  Health        %s://localhost:%s/health\n' "$scheme" "$API_PORT"
  printf '  Status        %s://localhost:%s/internal/status\n' "$scheme" "$API_PORT"
  if [ "$DEV" -eq 1 ]; then
    printf '  Vite UI       http://localhost:%s/ui/\n' "$UI_PORT"
  fi
  printf '\n'
  printf 'Admin token: %s\n' "$ADMIN_TOKEN"
  printf 'Data dir:    %s\n' "$DATA_DIR"
  printf 'Git dir:     %s\n' "$GIT_DIR"
  printf 'Object store bucket: %s\n' "$BLEEPHUB_OBJECT_S3_BUCKET"
  if [ -n "${BLEEPHUB_OBJECT_S3_ENDPOINT:-}" ]; then
    printf 'Object store endpoint: %s\n' "$BLEEPHUB_OBJECT_S3_ENDPOINT"
  fi
  printf 'Server log:  %s\n' "$SERVER_LOG"
  if [ "$DEV" -eq 1 ]; then
    printf 'UI log:      %s\n' "$UI_LOG"
  fi
  printf '\n'
  printf 'Stop with: %s stop\n' "$0"

  if [ "$TLS" -eq 1 ]; then
    printf '\n'
    printf 'Note: the self-signed cert at %s must be trusted by your system/keychain\n' "$TLS_DIR/bph.crt"
    printf 'before tools like gh or a browser accept it.\n'
  fi
}

stop() {
  SERVER_PID=""
  UI_PID=""
  read_pids
  if is_running "$SERVER_PID"; then
    printf '▸ Stopping bleephub-server (PID %s)…\n' "$SERVER_PID"
    kill "$SERVER_PID" >/dev/null 2>&1 || true
  fi
  if is_running "$UI_PID"; then
    printf '▸ Stopping Vite dev server (PID %s)…\n' "$UI_PID"
    kill "$UI_PID" >/dev/null 2>&1 || true
  fi
  rm -f "$PID_FILE"
}

status() {
  SERVER_PID=""
  UI_PID=""
  read_pids
  printf 'bleephub-server: %s\n' "$(is_running "$SERVER_PID" && echo "running (PID $SERVER_PID)" || echo 'not running')"
  printf 'Vite dev server: %s\n' "$(is_running "$UI_PID" && echo "running (PID $UI_PID)" || echo 'not running')"
  printf 'API port:        %s\n' "$API_PORT"
  printf 'Data dir:        %s\n' "$DATA_DIR"
  printf 'Git dir:         %s\n' "$GIT_DIR"
}

logs_cmd() {
  local follow=""
  if [ "${1:-}" = "-f" ]; then
    follow="-f"
  fi
  if [ -f "$SERVER_LOG" ]; then
    tail $follow "$SERVER_LOG"
  else
    printf 'No server log at %s\n' "$SERVER_LOG"
  fi
}

clean() {
  stop
  local paths=("$DATA_DIR" "$GIT_DIR" "$SERVER_LOG" "$UI_LOG" "$PID_FILE")
  if [ -d "$LOCAL_ROOT" ]; then
    paths+=("$LOCAL_ROOT")
  fi

  if [ "$YES" -ne 1 ]; then
    printf 'This will remove:\n'
    for p in "${paths[@]}"; do
      printf '  %s\n' "$p"
    done
    printf 'Proceed? [y/N] '
    read -r answer
    if [ "$answer" != "y" ] && [ "$answer" != "Y" ]; then
      printf 'Aborted.\n'
      exit 0
    fi
  fi

  for p in "${paths[@]}"; do
    rm -rf "$p"
  done
  printf 'Cleaned local bleephub dev data.\n'
}

parse_global_flags() {
  local args=()
  while [ $# -gt 0 ]; do
    case "$1" in
      --dev) DEV=1 ;;
      --tls) TLS=1 ;;
      --no-build) BUILD=0 ;;
      --yes) YES=1 ;;
      -h|--help) usage; exit 0 ;;
      -*) die "unknown option: $1" ;;
      *) args+=("$1") ;;
    esac
    shift
  done
  set -- "${args[@]}"
  if [ $# -eq 0 ]; then
    usage
    exit 1
  fi
  CMD="$1"
  shift
}

parse_global_flags "$@"

case "$CMD" in
  start)
    start "$@"
    ;;
  stop)
    stop
    ;;
  restart)
    stop
    sleep 1
    start "$@"
    ;;
  status)
    status
    ;;
  logs)
    logs_cmd "$@"
    ;;
  clean)
    clean
    ;;
  *)
    usage
    exit 1
    ;;
esac
