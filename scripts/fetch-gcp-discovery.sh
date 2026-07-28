#!/usr/bin/env bash
# Vendor (or refresh) one Google API Discovery document into specs/cloud-api/gcp/.
#
# Discovery documents are the machine-readable source google.golang.org/api
# clients are generated from. They are served live (not versioned in git), and
# Google can serve several revisions concurrently. The fetch probes the
# endpoint repeatedly and records the newest document it observes.
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

PROBES="${GCP_DISCOVERY_PROBES:-3}"
if ! [[ "$PROBES" =~ ^[1-9][0-9]*$ ]]; then
  echo "error: GCP_DISCOVERY_PROBES must be a positive integer, got $PROBES" >&2
  exit 1
fi

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT
best=""
best_revision=""
echo "Fetching $URL ($PROBES probes)"
for ((probe = 1; probe <= PROBES; probe++)); do
  candidate="$tmpdir/probe-$probe.json"
  if ! curl -fsSL -H 'Cache-Control: no-cache' -o "$candidate" "$URL"; then
    echo "warning: probe $probe failed" >&2
    continue
  fi
  if ! jq -e '.discoveryVersion and .revision and (.resources or .methods)' "$candidate" >/dev/null 2>&1; then
    echo "warning: probe $probe did not return a Discovery document" >&2
    continue
  fi
  revision="$(jq -r .revision "$candidate")"
  if ! [[ "$revision" =~ ^[0-9]+$ ]]; then
    echo "warning: probe $probe returned invalid revision $revision" >&2
    continue
  fi
  if [ -z "$best_revision" ] || [ "$revision" -gt "$best_revision" ]; then
    best="$candidate"
    best_revision="$revision"
  fi
done
if [ -z "$best" ]; then
  echo "error: no probe returned a valid Discovery document" >&2
  exit 1
fi
REVISION="$best_revision"

gzip -9 -n -c "$best" > "$DEST"

bash "$ROOT/scripts/spec-sources-row.sh" "$SOURCES" "gcp" \
  "${NAME}-${VERSION}.discovery.json.gz" "$UPSTREAM_HOST" "$UPSTREAM_PATH" \
  "Apache-2.0" "revision $REVISION"

echo "Vendored $DEST (revision $REVISION, $(wc -c < "$DEST" | tr -d ' ') bytes gzipped)"
