#!/usr/bin/env bash
# Run jscpd (JavaScript/TypeScript copy-paste detector) on bleephub UI src.
# Threshold: 200 tokens; test files excluded from the count.
set -euo pipefail

cd ui/packages/bleephub

# Run jscpd on src/ only (min 200 tokens — eliminates pure structural
# React boilerplate from DataTable column definitions and tab panel setup).
set +e
out=$(npx --yes jscpd \
  --min-tokens 200 \
  --ignore-pattern "src/__tests__/**" \
  --reporters "console" \
  src 2>&1)
status=$?
set -e

if echo "$out" | grep -q "^Clone found"; then
  count=$(echo "$out" | grep -c "^Clone found" || true)
  echo "FAIL: bleephub jscpd found $count clone(s) above threshold (200 tokens):" >&2
  echo "$out" >&2
  exit 1
fi
if [ "$status" -ne 0 ]; then
  echo "FAIL: bleephub jscpd exited with status $status:" >&2
  echo "$out" >&2
  exit 1
fi
echo "bleephub-jscpd: OK (threshold: 200 tokens)"
