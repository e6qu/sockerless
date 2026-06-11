#!/usr/bin/env bash
# Vendor (or refresh) one Google API Discovery document into specs/cloud-api/gcp/.
#
# Discovery documents are the machine-readable source google.golang.org/api
# clients are generated from. They are served live (not versioned in git), so
# the document's own "revision" field is recorded as the pin in
# specs/cloud-api/gcp/SOURCES.md.
#
# Two upstream locations exist, selected explicitly (no fallback): most
# services serve their own document at
# https://<host>/$discovery/rest?version=<v>; a few (e.g. compute) are only
# on the central discovery index at
# https://www.googleapis.com/discovery/v1/apis/<name>/<version>/rest, and a
# few newer ones (e.g. run v2) are only per-host.
#
# Usage:
#   scripts/fetch-gcp-discovery.sh <service-host> <version> [<local-name>] [central]
#   scripts/fetch-gcp-discovery.sh storage.googleapis.com v1
#   scripts/fetch-gcp-discovery.sh run.googleapis.com v2 cloudrun
#   scripts/fetch-gcp-discovery.sh compute.googleapis.com v1 compute central
set -euo pipefail

if [ $# -lt 2 ] || [ $# -gt 4 ]; then
  echo "usage: $0 <service-host> <version> [<local-name>] [central]" >&2
  exit 2
fi

HOST="$1"
VERSION="$2"
NAME="${3:-${HOST%%.googleapis.com}}"
SOURCE="${4:-perhost}"

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DEST_DIR="$ROOT/specs/cloud-api/gcp"
DEST="$DEST_DIR/${NAME}-${VERSION}.discovery.json.gz"
SOURCES="$DEST_DIR/SOURCES.md"
mkdir -p "$DEST_DIR"

case "$SOURCE" in
  perhost)
    URL="https://$HOST/\$discovery/rest?version=$VERSION"
    UPSTREAM_HOST="$HOST"
    UPSTREAM_PATH="\$discovery/rest?version=$VERSION"
    ;;
  central)
    SVC="${HOST%%.googleapis.com}"
    URL="https://www.googleapis.com/discovery/v1/apis/$SVC/$VERSION/rest"
    UPSTREAM_HOST="www.googleapis.com"
    UPSTREAM_PATH="discovery/v1/apis/$SVC/$VERSION/rest"
    ;;
  *)
    echo "error: source must be 'perhost' or 'central', got '$SOURCE'" >&2
    exit 2
    ;;
esac

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
echo "Fetching $URL"
curl -fsSL -o "$tmp" "$URL"

if ! jq -e '.discoveryVersion and .revision and (.resources or .methods)' "$tmp" >/dev/null 2>&1; then
  echo "error: downloaded file is not a Discovery document" >&2
  exit 1
fi
REVISION="$(jq -r .revision "$tmp")"

gzip -9 -n -c "$tmp" > "$DEST"

bash "$ROOT/scripts/spec-sources-row.sh" "$SOURCES" "gcp" \
  "${NAME}-${VERSION}.discovery.json.gz" "$UPSTREAM_HOST" "$UPSTREAM_PATH" \
  "Apache-2.0" "revision $REVISION"

echo "Vendored $DEST (revision $REVISION, $(wc -c < "$DEST" | tr -d ' ') bytes gzipped)"
