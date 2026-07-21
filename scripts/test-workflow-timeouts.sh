#!/usr/bin/env bash
set -euo pipefail

root="$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)"
fixture="$(mktemp -d)"
trap 'rm -rf "$fixture"' EXIT

expect_pass() {
	if ! "$root/scripts/check-workflow-timeouts.sh" "$1" >/dev/null; then
		echo "expected workflow timeout fixture to pass: $1" >&2
		exit 1
	fi
}

expect_fail() {
	if "$root/scripts/check-workflow-timeouts.sh" "$1" >/dev/null 2>&1; then
		echo "expected workflow timeout fixture to fail: $1" >&2
		exit 1
	fi
}

mkdir -p "$fixture/pass" "$fixture/too-long" "$fixture/missing" "$fixture/matrix-too-long"
cat >"$fixture/pass/ci.yml" <<'YAML'
name: pass
jobs:
  direct:
    timeout-minutes: 15
    runs-on: ubuntu-latest
    steps: []
  matrix:
    timeout-minutes: ${{ matrix.timeout_minutes }}
    strategy:
      matrix:
        include:
          - timeout_minutes: 3
          - timeout_minutes: 15
    runs-on: ubuntu-latest
    steps: []
  reusable:
    uses: owner/repository/.github/workflows/reusable.yml@main
YAML
cat >"$fixture/too-long/ci.yml" <<'YAML'
name: too long
jobs:
  test:
    timeout-minutes: 16
    runs-on: ubuntu-latest
    steps: []
YAML
cat >"$fixture/missing/ci.yml" <<'YAML'
name: missing
jobs:
  test:
    runs-on: ubuntu-latest
    steps: []
YAML
cat >"$fixture/matrix-too-long/ci.yml" <<'YAML'
name: matrix too long
jobs:
  test:
    timeout-minutes: ${{ matrix.timeout_minutes }}
    strategy:
      matrix:
        include:
          - timeout_minutes: 20
    runs-on: ubuntu-latest
    steps: []
YAML

expect_pass "$fixture/pass"
expect_fail "$fixture/too-long"
expect_fail "$fixture/missing"
expect_fail "$fixture/matrix-too-long"
echo "workflow timeout fixture tests passed"
