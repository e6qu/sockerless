#!/usr/bin/env bash
# Reject newly-added test skips that hide missing tools/dependencies. Required
# tools must be installed by the test harness or fail loud; only platform/kernel
# capability gates should skip.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

range="${1:---cached}"
diff=$(git diff "$range" -- '*_test.go' 2>/dev/null || true)
if [[ -z "$diff" ]]; then
  exit 0
fi

matches=$(printf '%s\n' "$diff" \
  | grep -E '^\+[^+].*t\.Skipf?\(' \
  | grep -Ei 'not available|not found|not installed|missing|required.*(binary|tool|cli)|could not launch.*skipping|skipping.*(emulator|differential|oracle)' \
  || true)

if [[ -n "$matches" ]]; then
  cat >&2 <<'MSG'
New skip-if-tool/dependency-absent test code is not allowed.

Required tools must be installed by TestMain/harness setup or fail loud with
t.Fatal/log.Fatal. Kernel/platform capability gates are allowed, but phrase them
as capability/platform gates rather than missing-tool skips.
MSG
  printf '%s\n' "$matches" >&2
  exit 1
fi

exit 0
