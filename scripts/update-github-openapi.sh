#!/usr/bin/env bash
# Refresh the vendored GitHub OpenAPI description used by bleephub's
# API-definition fidelity test (bleephub/gh_api_definition_test.go).
#
# We vendor the official github/rest-api-description "api.github.com" bundled
# description (gzipped) so the test is hermetic — CI never downloads it.
# Run this to pull a newer upstream snapshot.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DEST="$ROOT/bleephub/testdata/github-openapi.json.gz"
VERSION_FILE="$ROOT/bleephub/testdata/github-openapi.VERSION"
URL="https://raw.githubusercontent.com/github/rest-api-description/main/descriptions/api.github.com/api.github.com.json"

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT

echo "Fetching $URL"
curl -sSL -o "$tmp" "$URL"

if ! jq -e '.openapi and .paths' "$tmp" >/dev/null 2>&1; then
  echo "error: downloaded file is not a valid OpenAPI document" >&2
  exit 1
fi

ver="$(jq -r '.info.version' "$tmp")"
gzip -c "$tmp" > "$DEST"

cat > "$VERSION_FILE" <<EOF
source: $URL
openapi info.version: $ver
vendored: github.com/github/rest-api-description (api.github.com bundled description)
refresh: scripts/update-github-openapi.sh
EOF

echo "Vendored $DEST (info.version $ver, $(wc -c < "$DEST") bytes gzipped)"
