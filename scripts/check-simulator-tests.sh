#!/usr/bin/env bash
# check-simulator-tests.sh — enforces the testing contract:
# every simulator code change must ship with matching SDK + CLI +
# terraform-test coverage. Runs as a pre-commit hook.
#
# Contract:
#   - Any new `r.Register("<operation>", ...)` or
#     `r.RegisterVersioned("<api-version>", "<operation>", ...)` line added under
#     simulators/<cloud>/ (outside *_test.go / docs / README) must be
#     referenced in a *test file* within the same commit under
#     simulators/<cloud>/{sdk-tests,cli-tests,terraform-tests}.
#   - The same applies to newly-added `(srv|mux).HandleFunc("<METHOD> <path>", ...)`
#     route mounts: an SDK/CLI/terraform-exercisable REST endpoint mounted that
#     way must ship with a test that references its route path — or be listed on
#     tests-exempt.txt (for internal runtime/metadata/dashboard routes no
#     SDK/CLI/terraform surface exposes). The reference token is the route's
#     literal path (e.g. `/apps/{appId}/domains`).
#   - An operation, or a HandleFunc route's literal "<METHOD> <path>" token, can
#     be placed on simulators/<cloud>/tests-exempt.txt to opt out (e.g. Lambda
#     Runtime API routes internal to the container lifecycle, IMDS metadata
#     routes, or sim dashboard routes).
#
# Diff scoping: only NEWLY-ADDED ('+') Register/RegisterVersioned/HandleFunc
# lines in the diff are enforced. The hundreds of pre-existing HandleFunc routes
# are grandfathered — they are never retroactively required to grow tests.
#
# Usage:
#   scripts/check-simulator-tests.sh           # check staged changes
#   scripts/check-simulator-tests.sh --ref HEAD^  # check between ref and HEAD
set -euo pipefail

ref="${1:-}"
if [[ "$ref" == "--ref" && -n "${2:-}" ]]; then
    staged_range="$2..HEAD"
else
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

regex_escape() { printf '%s' "$1" | sed 's|[][\\.*^$/]|\\&|g'; }

# Added ('+') lines across changed simulator .go files.
added_lines=$(git diff "$staged_range" -- 'simulators/*.go' 2>/dev/null \
    | grep -E '^\+[^+]' || true)

# Newly-registered operations (Register / RegisterVersioned).
newly_registered=$(printf '%s\n' "$added_lines" \
    | grep -E 'r\.Register(Versioned)?\s*\(' \
    | sed -nE \
        -e 's/.*r\.Register\s*\(\s*"([^"]+)".*/\1/p' \
        -e 's/.*r\.RegisterVersioned\s*\(\s*[^,]+\s*,\s*"([^"]+)".*/\1/p' \
    | sort -u || true)

# Newly-added HandleFunc route mounts whose first argument is a literal
# "METHOD /path" string. The route token is "METHOD /literal-path-prefix":
# the literal portion of the path up to the first concatenation (closing quote),
# which is robust to forms like  "POST /"+cfAPIVersion+"/distribution".
# Routes whose first argument is not a "METHOD /…" literal (rare) are ignored —
# they cannot be reliably keyed, and the Register/op path already covers SDK ops.
handlefunc_routes=$(printf '%s\n' "$added_lines" \
    | grep -E '(srv|mux)\.HandleFunc\s*\(\s*"[A-Z]+ /' \
    | sed -nE 's/.*HandleFunc\s*\(\s*"([A-Z]+ \/[^"]*)".*/\1/p' \
    | sed -E 's/[[:space:]]+$//' \
    | sort -u || true)

if [[ -z "$newly_registered" && -z "$handlefunc_routes" ]]; then
    exit 0
fi

# Collect test-file changes by cloud.
get_tests_for_cloud() {
    local cloud="$1"
    git diff --name-only "$staged_range" 2>/dev/null \
        | grep -E "^simulators/${cloud}/(sdk-tests|cli-tests|terraform-tests)/" \
        || true
}

cloud_of() { echo "$1" | sed -nE 's|^simulators/([^/]+)/.*|\1|p'; }

changed_sim_files=$(git diff --name-only "$staged_range" 2>/dev/null | grep -E '^simulators/' || true)

# Cloud of a registered op: the file whose added diff defines its Register line.
op_to_cloud() {
    local op escaped_op f
    op="$1"; escaped_op="$(regex_escape "$op")"
    for f in $changed_sim_files; do
        if git diff "$staged_range" -- "$f" 2>/dev/null | grep -E '^\+[^+]' \
                | grep -qE "(r\.Register\s*\(\s*\"${escaped_op}\")|(r\.RegisterVersioned\s*\(\s*[^,]+\s*,\s*\"${escaped_op}\")"; then
            cloud_of "$f"; return
        fi
    done
}

# Cloud of a HandleFunc route: the file whose added diff mounts it.
route_to_cloud() {
    local route escaped f
    route="$1"; escaped="$(regex_escape "$route")"
    for f in $changed_sim_files; do
        if git diff "$staged_range" -- "$f" 2>/dev/null | grep -E '^\+[^+]' \
                | grep -qE "(srv|mux)\.HandleFunc\s*\(\s*\"${escaped}"; then
            cloud_of "$f"; return
        fi
    done
}

# An op is "referenced" only in a real call/assertion context on an added test
# line — not a bare comment/string mention:
#   SDK Go:  .<Op>(            CLI: "<kebab-op>"            literal: "<Op>"
op_referenced_in_tests() {
    local short tests_changed kebab tf
    short="$1"; shift; tests_changed="$*"
    kebab=$(printf '%s' "$short" \
        | sed -E 's/([a-z0-9])([A-Z])/\1-\2/g; s/([A-Z]+)([A-Z][a-z])/\1-\2/g' \
        | tr '[:upper:]' '[:lower:]')
    local esc_short esc_kebab
    esc_short="$(regex_escape "$short")"; esc_kebab="$(regex_escape "$kebab")"
    for tf in $tests_changed; do
        if git diff "$staged_range" -- "$tf" 2>/dev/null | grep -E '^\+' \
                | grep -qE "\.${esc_short}\(|\"${esc_kebab}\"|\"${esc_short}\""; then
            return 0
        fi
    done
    return 1
}

# A route is "referenced" when its literal path appears on an added test line,
# OR — for routes whose final path segment is a CamelCase operation name (the
# SDK-op-in-path form, e.g. /operation/PutDashboard, where the SDK/CLI client
# invokes the op by name rather than POSTing the literal wire path) — when that
# op is referenced in a call/assertion (.PutDashboard( / "put-dashboard").
route_referenced_in_tests() {
    local route tests_changed path last_seg tf
    route="$1"; shift; tests_changed="$*"
    path="${route#* }"
    for tf in $tests_changed; do
        if git diff "$staged_range" -- "$tf" 2>/dev/null | grep -E '^\+' \
                | grep -qF "$path" 2>/dev/null; then
            return 0
        fi
    done
    # Trailing CamelCase segment ⇒ SDK op name; accept a call/assertion reference.
    last_seg="${path##*/}"
    if printf '%s' "$last_seg" | grep -qE '^[A-Z][a-zA-Z0-9]+$'; then
        op_referenced_in_tests "$last_seg" "$tests_changed" && return 0
    fi
    return 1
}

fail=0

for op in $newly_registered; do
    cloud=$(op_to_cloud "$op")
    if [[ -z "$cloud" ]]; then
        echo "[simulator-tests] FAIL: registered op \"$op\" — could not determine its cloud (no matching added Register line found in simulators/<cloud>/). Ops must not slip silently; define it in a simulators/<cloud>/*.go file in this commit." >&2
        fail=1; continue
    fi

    exempt_file="simulators/${cloud}/tests-exempt.txt"
    if [[ -f "$exempt_file" ]] && grep -qxF "$op" "$exempt_file"; then continue; fi

    short=$(printf '%s' "$op" | sed 's|.*\.||')
    tests_changed=$(get_tests_for_cloud "$cloud")
    if [[ -z "$tests_changed" ]]; then
        echo "[simulator-tests] FAIL: op \"$op\" ($cloud) — no test file changes under simulators/$cloud/{sdk-tests,cli-tests,terraform-tests}/ in this commit." >&2
        fail=1; continue
    fi

    if ! op_referenced_in_tests "$short" "$tests_changed"; then
        echo "[simulator-tests] FAIL: op \"$op\" ($cloud) — changed test files don't reference \"$short\" in a call/assertion (.$short( for SDK, \"${short}\"/kebab for CLI). Add a real SDK/CLI/terraform test, or add it to simulators/$cloud/tests-exempt.txt." >&2
        fail=1
    fi
done

while IFS= read -r route; do
    [[ -z "$route" ]] && continue
    cloud=$(route_to_cloud "$route")
    if [[ -z "$cloud" ]]; then
        echo "[simulator-tests] FAIL: HandleFunc route \"$route\" — could not determine its cloud. Routes must not slip silently." >&2
        fail=1; continue
    fi

    exempt_file="simulators/${cloud}/tests-exempt.txt"
    if [[ -f "$exempt_file" ]] && grep -qxF "$route" "$exempt_file"; then continue; fi

    tests_changed=$(get_tests_for_cloud "$cloud")
    if [[ -z "$tests_changed" ]]; then
        echo "[simulator-tests] FAIL: HandleFunc route \"$route\" ($cloud) — no test file changes under simulators/$cloud/{sdk-tests,cli-tests,terraform-tests}/ in this commit. If the route is internal (runtime/metadata/dashboard) and not SDK/CLI/terraform-exercisable, add the exact \"$route\" token to simulators/$cloud/tests-exempt.txt." >&2
        fail=1; continue
    fi

    if ! route_referenced_in_tests "$route" "$tests_changed"; then
        echo "[simulator-tests] FAIL: HandleFunc route \"$route\" ($cloud) — changed test files don't reference its path. Add an SDK/CLI/terraform test exercising the route, or add the exact \"$route\" token to simulators/$cloud/tests-exempt.txt." >&2
        fail=1
    fi
done <<< "$handlefunc_routes"

[[ $fail -ne 0 ]] && exit 1
exit 0
