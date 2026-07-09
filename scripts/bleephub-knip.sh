#!/usr/bin/env bash
# Run knip (TypeScript dead-exports checker) on the bleephub UI package.
set -euo pipefail

cd ui/packages/bleephub

out=$(npx knip 2>&1)
# knip exits 0 on success; strip the deprecation warning line before checking
filtered=$(echo "$out" | grep -v "DeprecationWarning\|module.register\|trace-deprecation\|trace-warnings\|registerHooks\|node:process" || true)
if [[ -n "$filtered" ]]; then
  echo "FAIL: bleephub knip found dead exports/files:" >&2
  echo "$filtered" >&2
  exit 1
fi
echo "bleephub-knip: OK"
