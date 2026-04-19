#!/usr/bin/env bash
# check-simulator-tests.sh — enforces the Phase-86 testing contract:
# every simulator code change must ship with matching SDK + CLI +
# terraform-test coverage. Runs as a pre-commit hook.
#
# Contract:
#   - Any new `r.Register("<operation>", ...)` line added under
#     simulators/<cloud>/ (outside *_test.go / docs / README) must be
#     referenced in a *test file* within the same commit under
#     simulators/<cloud>/{sdk-tests,cli-tests,terraform-tests}.
#   - An operation can be placed on simulators/<cloud>/tests-exempt.txt
#     to opt out (e.g. Lambda Runtime API routes internal to the
#     container lifecycle that no SDK/CLI/terraform surface exposes).
#
# Usage:
#   scripts/check-simulator-tests.sh           # check staged changes
#   scripts/check-simulator-tests.sh --ref HEAD^  # check between ref and HEAD
set -euo pipefail

ref="${1:-}"
if [[ "$ref" == "--ref" && -n "${2:-}" ]]; then
    diff_range="$2..HEAD"
    staged_range="$diff_range"
else
    diff_range="--cached"
    staged_range="--cached"
fi

# Collect changed simulator .go files that aren't test files
changed_go=$(git diff --name-only "$staged_range" 2>/dev/null \
    | grep -E '^simulators/(aws|gcp|azure)/[^/]*\.go$' \
    | grep -vE '(_test\.go|/docs/|/README\.md|/go\.mod|/go\.sum)$' \
    || true)

if [[ -z "$changed_go" ]]; then
    exit 0
fi

# Collect newly-registered operations from the diff.
# Matches: r.Register("Service.Operation", handlerName)
# Captures the operation name (between quotes).
newly_registered=$(git diff "$staged_range" -- 'simulators/*.go' 2>/dev/null \
    | grep -E '^\+[^+].*r\.Register\s*\(' \
    | sed -nE 's/.*r\.Register\s*\(\s*"([^"]+)".*/\1/p' \
    | sort -u || true)

if [[ -z "$newly_registered" ]]; then
    exit 0
fi

# Collect test-file changes by cloud.
get_tests_for_cloud() {
    local cloud="$1"
    git diff --name-only "$staged_range" 2>/dev/null \
        | grep -E "^simulators/${cloud}/(sdk-tests|cli-tests|terraform-tests)/" \
        || true
}

# Determine the cloud a file belongs to.
cloud_of() {
    local path="$1"
    echo "$path" | sed -nE 's|^simulators/([^/]+)/.*|\1|p'
}

# Determine which cloud each newly-registered op belongs to by finding
# the defining file in the diff. If the op appears under
# simulators/aws/ecr.go, it's "aws" regardless of name.
op_to_cloud() {
    local op="$1"
    # Search the staged diff for the file containing this r.Register line.
    local files
    files=$(git diff --name-only "$staged_range" 2>/dev/null | grep -E '^simulators/' || true)
    for f in $files; do
        if git diff "$staged_range" -- "$f" 2>/dev/null \
                | grep -qE "^\+[^+].*r\.Register\s*\(\s*\"$(printf '%s' "$op" | sed 's|[][\\.*^$/]|\\&|g')\""; then
            cloud_of "$f"
            return
        fi
    done
}

fail=0
for op in $newly_registered; do
    cloud=$(op_to_cloud "$op")
    if [[ -z "$cloud" ]]; then
        continue
    fi

    # Check the opt-out manifest.
    exempt_file="simulators/${cloud}/tests-exempt.txt"
    if [[ -f "$exempt_file" ]] && grep -qxF "$op" "$exempt_file"; then
        continue
    fi

    # Look for the op name (or the operation short name after the last dot)
    # in any changed test file under sdk-tests / cli-tests / terraform-tests
    # for this cloud.
    short=$(printf '%s' "$op" | sed 's|.*\.||')
    tests_changed=$(get_tests_for_cloud "$cloud")
    if [[ -z "$tests_changed" ]]; then
        echo "[simulator-tests] FAIL: registered op \"$op\" ($cloud) — no test file changes under simulators/$cloud/{sdk-tests,cli-tests,terraform-tests}/ in this commit." >&2
        fail=1
        continue
    fi

    # Verify at least one changed test file references the op name or short name.
    matched=0
    for tf in $tests_changed; do
        if git diff "$staged_range" -- "$tf" 2>/dev/null \
                | grep -Eq "(^\+.*$(printf '%s' "$short" | sed 's|[][\\.*^$/]|\\&|g'))"; then
            matched=1
            break
        fi
    done
    if [[ $matched -eq 0 ]]; then
        echo "[simulator-tests] FAIL: registered op \"$op\" ($cloud) — changed test files don't reference \"$short\". Add it to sdk-tests, cli-tests, or terraform-tests — or to simulators/$cloud/tests-exempt.txt if it's intentionally out of SDK/CLI/terraform scope." >&2
        fail=1
    fi
done

if [[ $fail -ne 0 ]]; then
    exit 1
fi

exit 0
