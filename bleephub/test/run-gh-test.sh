#!/usr/bin/env bash
set -euo pipefail

PASS=0
FAIL=0
ERRORS=""

log() { echo "=== [gh-test] $*"; }
pass() { log "PASS: $1"; PASS=$((PASS + 1)); }
fail() {
    log "FAIL: $1"
    FAIL=$((FAIL + 1))
    ERRORS="$ERRORS\n  - $1"
}

assert_eq() {
    local desc="$1" expected="$2" actual="$3"
    if [ "$expected" = "$actual" ]; then
        pass "$desc"
    else
        fail "$desc (expected '$expected', got '$actual')"
    fi
}

assert_contains() {
    local desc="$1" haystack="$2" needle="$3"
    if echo "$haystack" | grep -qF "$needle"; then
        pass "$desc"
    else
        fail "$desc (expected to contain '$needle')"
    fi
}

assert_not_empty() {
    local desc="$1" value="$2"
    if [ -n "$value" ]; then
        pass "$desc"
    else
        fail "$desc (expected non-empty)"
    fi
}

# --- Generate self-signed TLS certificates ---
log "Generating TLS certificates..."
mkdir -p /tmp/tls
openssl req -x509 -newkey rsa:2048 -keyout /tmp/tls/ca.key -out /tmp/tls/ca.crt \
    -days 1 -nodes -subj "/CN=bleephub-ca" 2>/dev/null
openssl req -newkey rsa:2048 -keyout /tmp/tls/server.key -out /tmp/tls/server.csr \
    -nodes -subj "/CN=localhost" 2>/dev/null
cat > /tmp/tls/ext.cnf <<EOF
authorityKeyIdentifier=keyid,issuer
basicConstraints=CA:FALSE
subjectAltName=DNS:localhost,IP:127.0.0.1
EOF
openssl x509 -req -in /tmp/tls/server.csr -CA /tmp/tls/ca.crt -CAkey /tmp/tls/ca.key \
    -CAcreateserial -out /tmp/tls/server.crt -days 1 -extfile /tmp/tls/ext.cnf 2>/dev/null

# Trust the CA system-wide
cp /tmp/tls/ca.crt /usr/local/share/ca-certificates/bleephub-ca.crt
update-ca-certificates 2>/dev/null || true

# For Go/git to trust it too
export SSL_CERT_FILE=/tmp/tls/ca.crt
export GIT_SSL_CAINFO=/tmp/tls/ca.crt

# --- Start bleephub on port 443 with TLS ---
log "Starting bleephub..."
export BPH_TLS_CERT=/tmp/tls/server.crt
export BPH_TLS_KEY=/tmp/tls/server.key
bleephub -addr :443 --log-level debug > /tmp/bleephub.log 2>&1 &
BPH_PID=$!

# Wait for server
for i in $(seq 1 30); do
    if curl -sSk https://localhost/health >/dev/null 2>&1; then
        break
    fi
    sleep 0.5
done

# Verify server is running
if ! curl -sSk https://localhost/health >/dev/null 2>&1; then
    log "FATAL: bleephub did not start"
    exit 1
fi
log "bleephub running on https://localhost:443"

# --- Configure git ---
git config --global user.email "test@bleephub.local"
git config --global user.name "Test User"
git config --global init.defaultBranch main

# Default token
TOKEN="ghp_0000000000000000000000000000000000000000"
BASE="https://localhost"
HOST="localhost"

# --- Authenticate gh CLI against bleephub ---
# gh CLI gates all calls on "you must be logged in to some host". Login it
# against bleephub as a GHES host so high-level commands (gh repo create,
# gh issue create, gh pr create, gh release create, ...) target bleephub.
# We set GH_TOKEN to satisfy the default-host check AND `gh auth login`
# the bleephub host explicitly with the same token.
export GH_TOKEN="$TOKEN"
export GH_HOST="$HOST"
# Login the host so gh's host config has bleephub as a known GHES.
echo "$TOKEN" | gh auth login --hostname "$HOST" --with-token >/dev/null 2>&1 || true
gh config set -h "$HOST" git_protocol https >/dev/null 2>&1 || true

# `api` for endpoints `gh` doesn't expose as a high-level command
# (apps/{slug}, /applications/{cid}/token, suspend, etc.). For the
# happy-path repo/issue/PR/release surface, use real `gh repo create`,
# `gh issue create`, `gh pr create`, `gh release create` below.
api() {
    gh api -H "Authorization: token $TOKEN" -H "Accept: application/vnd.github+json" "$@"
}

# ============================================================
# Test: API Root
# ============================================================
log "Test: API Root"
ROOT=$(api "$BASE/api/v3/")
assert_contains "API root has current_user_url" "$ROOT" "current_user_url"

# ============================================================
# Test: Viewer (current user)
# ============================================================
log "Test: Viewer"
USER=$(api "$BASE/api/v3/user")
LOGIN=$(echo "$USER" | jq -r '.login')
assert_eq "viewer login" "admin" "$LOGIN"

# ============================================================
# Test: GraphQL viewer
# ============================================================
log "Test: GraphQL viewer"
GQL=$(api "$BASE/api/graphql" -f query='{ viewer { login } }')
GQL_LOGIN=$(echo "$GQL" | jq -r '.data.viewer.login')
assert_eq "graphql viewer login" "admin" "$GQL_LOGIN"

# ============================================================
# Test: Create repo via real `gh repo create`
# ============================================================
log "Test: gh repo create"
# gh repo create posts to /user/repos with a JSON body matching real GitHub.
# --public sends private=false; --description maps to description.
if ! gh repo create gh-test-repo --public --description "GH CLI test" >/dev/null 2>&1; then
    fail "gh repo create failed"
else
    pass "gh repo create"
fi
REPO=$(api "$BASE/api/v3/repos/admin/gh-test-repo")
REPO_NAME=$(echo "$REPO" | jq -r '.name')
assert_eq "repo name" "gh-test-repo" "$REPO_NAME"
REPO_FULLNAME=$(echo "$REPO" | jq -r '.full_name')
assert_eq "repo full_name" "admin/gh-test-repo" "$REPO_FULLNAME"

# Verify permissions in response
PERMS_ADMIN=$(echo "$REPO" | jq -r '.permissions.admin')
assert_eq "repo permissions.admin" "true" "$PERMS_ADMIN"

# ============================================================
# Test: List repos via real `gh repo list`
# ============================================================
log "Test: gh repo list"
# Without --json gh uses REST. With --json it uses GraphQL (separate
# parity surface). REST path is the minimum that must work.
if gh repo list admin >/dev/null 2>&1; then
    pass "gh repo list"
else
    fail "gh repo list returned non-zero"
fi

# ============================================================
# Test: View repo via real `gh repo view` (REST path, no --json)
# ============================================================
log "Test: gh repo view"
# Without --json gh uses REST. With --json it uses GraphQL — that's a
# separate parity surface (gh's GraphQL field names map onto bleephub's
# schema). REST path is the minimum that must work.
if gh repo view admin/gh-test-repo >/dev/null 2>&1; then
    pass "gh repo view"
else
    fail "gh repo view returned non-zero"
fi

# ============================================================
# Test: Get repo
# ============================================================
log "Test: Get repo"
REPO_GET=$(api "$BASE/api/v3/repos/admin/gh-test-repo")
REPO_GET_NAME=$(echo "$REPO_GET" | jq -r '.name')
assert_eq "get repo name" "gh-test-repo" "$REPO_GET_NAME"

# ============================================================
# Test: Create label
# ============================================================
log "Test: Create label"
LABEL=$(api "$BASE/api/v3/repos/admin/gh-test-repo/labels" -f name=bug -f color=d73a4a -f description="Something broken")
LABEL_NAME=$(echo "$LABEL" | jq -r '.name')
assert_eq "label name" "bug" "$LABEL_NAME"

# ============================================================
# Test: List labels
# ============================================================
log "Test: List labels"
LABELS=$(api "$BASE/api/v3/repos/admin/gh-test-repo/labels")
LABEL_COUNT=$(echo "$LABELS" | jq 'length')
if [ "$LABEL_COUNT" -ge 1 ]; then
    pass "list labels returns >= 1"
else
    fail "list labels returned $LABEL_COUNT"
fi

# ============================================================
# Test: Create issue via real `gh issue create`
# ============================================================
log "Test: gh issue create"
# Real gh exits 0 when the issue is created. We verify by GETting the
# issue via REST afterwards rather than parsing gh's URL output (which
# varies across gh versions and Host configs).
if ! gh issue create --repo admin/gh-test-repo --title "GH CLI issue" --body "Testing via real gh" >/dev/null 2>&1; then
    fail "gh issue create returned non-zero"
else
    pass "gh issue create exited 0"
fi
ISSUE_GET=$(api "$BASE/api/v3/repos/admin/gh-test-repo/issues/1")
ISSUE_NUM=$(echo "$ISSUE_GET" | jq -r '.number')
assert_eq "issue 1 exists after gh issue create" "1" "$ISSUE_NUM"
ISSUE_TITLE=$(echo "$ISSUE_GET" | jq -r '.title')
assert_eq "issue 1 title after gh issue create" "GH CLI issue" "$ISSUE_TITLE"
ISSUE_STATE=$(echo "$ISSUE_GET" | jq -r '.state')
assert_eq "issue 1 state after gh issue create" "open" "$ISSUE_STATE"

# ============================================================
# Test: View issue via real `gh issue view` (REST-backed, --json optional)
# ============================================================
log "Test: gh issue view"
# `gh issue view N --repo …` uses the REST API directly; --json args go
# through GraphQL on real GH. We test the REST-only path here by NOT
# passing --json — gh prints a human-readable summary on success.
if gh issue view 1 --repo admin/gh-test-repo >/dev/null 2>&1; then
    pass "gh issue view"
else
    fail "gh issue view returned non-zero"
fi

# ============================================================
# Test: List issues via real `gh issue list` (REST-backed)
# ============================================================
log "Test: gh issue list"
# Same as above — without --json gh uses REST.
if gh issue list --repo admin/gh-test-repo >/dev/null 2>&1; then
    pass "gh issue list"
else
    fail "gh issue list returned non-zero"
fi

# ============================================================
# Test: Close issue
# ============================================================
log "Test: Close issue"
CLOSED=$(api -X PATCH "$BASE/api/v3/repos/admin/gh-test-repo/issues/1" -f state=closed)
CLOSED_STATE=$(echo "$CLOSED" | jq -r '.state')
assert_eq "closed issue state" "closed" "$CLOSED_STATE"

# ============================================================
# Test: Reopen issue
# ============================================================
log "Test: Reopen issue"
REOPENED=$(api -X PATCH "$BASE/api/v3/repos/admin/gh-test-repo/issues/1" -f state=open)
REOPENED_STATE=$(echo "$REOPENED" | jq -r '.state')
assert_eq "reopened issue state" "open" "$REOPENED_STATE"

# ============================================================
# Test: Create pull request
# ============================================================
log "Test: Create pull request"
PR=$(api "$BASE/api/v3/repos/admin/gh-test-repo/pulls" -f title="GH CLI PR" -f head=feature -f base=main -f body="Test PR")
PR_NUM=$(echo "$PR" | jq -r '.number')
assert_eq "PR number" "2" "$PR_NUM"
PR_STATE=$(echo "$PR" | jq -r '.state')
assert_eq "PR state" "open" "$PR_STATE"

# ============================================================
# Test: List pull requests
# ============================================================
log "Test: List PRs"
PRS=$(api "$BASE/api/v3/repos/admin/gh-test-repo/pulls")
PR_COUNT=$(echo "$PRS" | jq 'length')
if [ "$PR_COUNT" -ge 1 ]; then
    pass "list PRs returns >= 1"
else
    fail "list PRs returned $PR_COUNT"
fi

# ============================================================
# Test: Get pull request
# ============================================================
log "Test: Get PR"
PR_GET=$(api "$BASE/api/v3/repos/admin/gh-test-repo/pulls/2")
PR_GET_TITLE=$(echo "$PR_GET" | jq -r '.title')
assert_eq "get PR title" "GH CLI PR" "$PR_GET_TITLE"

# ============================================================
# Test: Create PR review (approve)
# ============================================================
log "Test: PR review"
REVIEW=$(api "$BASE/api/v3/repos/admin/gh-test-repo/pulls/2/reviews" -f body=LGTM -f event=APPROVE)
REVIEW_STATE=$(echo "$REVIEW" | jq -r '.state')
assert_eq "review state" "APPROVED" "$REVIEW_STATE"

# ============================================================
# Test: Merge PR
# ============================================================
log "Test: Merge PR"
MERGE=$(api -X PUT "$BASE/api/v3/repos/admin/gh-test-repo/pulls/2/merge" -f merge_method=merge)
MERGED=$(echo "$MERGE" | jq -r '.merged')
assert_eq "PR merged" "true" "$MERGED"

# ============================================================
# Test: GraphQL repository query
# ============================================================
log "Test: GraphQL repo query"
GQL_REPO=$(api "$BASE/api/graphql" -f query='{repository(owner:"admin",name:"gh-test-repo"){name,isPrivate}}')
GQL_REPO_NAME=$(echo "$GQL_REPO" | jq -r '.data.repository.name')
assert_eq "graphql repo name" "gh-test-repo" "$GQL_REPO_NAME"

# ============================================================
# Test: GraphQL issues query
# ============================================================
log "Test: GraphQL issues query"
GQL_ISSUES=$(api "$BASE/api/graphql" -f query='{repository(owner:"admin",name:"gh-test-repo"){issues(first:10,states:[OPEN]){totalCount}}}')
GQL_ISSUES_COUNT=$(echo "$GQL_ISSUES" | jq -r '.data.repository.issues.totalCount')
if [ "$GQL_ISSUES_COUNT" -ge 1 ]; then
    pass "graphql issues totalCount >= 1"
else
    fail "graphql issues totalCount = $GQL_ISSUES_COUNT"
fi

# ============================================================
# Test: GraphQL PRs query (merged)
# ============================================================
log "Test: GraphQL PRs query"
GQL_PRS=$(api "$BASE/api/graphql" -f query='{repository(owner:"admin",name:"gh-test-repo"){pullRequests(first:10,states:[MERGED]){totalCount}}}')
GQL_PRS_COUNT=$(echo "$GQL_PRS" | jq -r '.data.repository.pullRequests.totalCount')
assert_eq "graphql merged PRs" "1" "$GQL_PRS_COUNT"

# ============================================================
# Test: Rate limit endpoint
# ============================================================
log "Test: Rate limit"
RATE=$(api "$BASE/api/v3/rate_limit")
RATE_LIMIT=$(echo "$RATE" | jq -r '.resources.core.limit')
assert_eq "rate limit core.limit" "5000" "$RATE_LIMIT"

# ============================================================
# Test: Org lifecycle (via API)
# ============================================================
log "Test: Create org"
ORG=$(api "$BASE/api/v3/user/orgs" -f login=gh-test-org -f name="Test Org")
ORG_LOGIN=$(echo "$ORG" | jq -r '.login')
assert_eq "org login" "gh-test-org" "$ORG_LOGIN"

log "Test: List orgs"
ORGS=$(api "$BASE/api/v3/user/orgs")
ORG_COUNT=$(echo "$ORGS" | jq 'length')
if [ "$ORG_COUNT" -ge 1 ]; then
    pass "list orgs returns >= 1"
else
    fail "list orgs returned $ORG_COUNT"
fi

# ============================================================
# Test: Pagination (Link header)
# ============================================================
log "Test: Pagination"
# Create a few more issues for pagination testing
api "$BASE/api/v3/repos/admin/gh-test-repo/issues" -f title="PG issue 2" >/dev/null
api "$BASE/api/v3/repos/admin/gh-test-repo/issues" -f title="PG issue 3" >/dev/null

HEADERS=$(curl -sSk -I -H "Authorization: token $TOKEN" "$BASE/api/v3/repos/admin/gh-test-repo/issues?per_page=1")
if echo "$HEADERS" | grep -qi "^link:"; then
    pass "pagination Link header present"
else
    fail "pagination Link header missing"
fi

# ============================================================
# Test: Content-Type charset
# ============================================================
log "Test: Content-Type charset"
CT=$(curl -sSk -I -H "Authorization: token $TOKEN" "$BASE/api/v3/user" | grep -i "^content-type:" | head -1)
if echo "$CT" | grep -qi "charset=utf-8"; then
    pass "Content-Type has charset=utf-8"
else
    fail "Content-Type missing charset: $CT"
fi

# ============================================================
# Test: 422 error format
# ============================================================
log "Test: 422 error format"
ERR422=$(curl -sSk -X POST -H "Authorization: token $TOKEN" -H "Content-Type: application/json" \
    -d '{"name":""}' "$BASE/api/v3/user/repos" || true)
ERR_MSG=$(echo "$ERR422" | jq -r '.message')
assert_eq "422 message" "Validation Failed" "$ERR_MSG"
ERR_ARRAY=$(echo "$ERR422" | jq -r '.errors | length')
if [ "$ERR_ARRAY" -ge 1 ]; then
    pass "422 errors array present"
else
    fail "422 errors array missing"
fi

# ============================================================
# Phase 153 — GitHub Apps + OAuth Apps parity tests
# ============================================================
log "Phase 153: GitHub Apps + OAuth Apps surface"

# Create a GitHub App with explicit permissions + events
APP=$(api "$BASE/api/v3/bleephub/apps" -f name="Parity App" -f description="Phase 153 test" \
    -f 'permissions[issues]=write' -f 'permissions[checks]=write' \
    -f 'events[]=push' -f 'events[]=installation')
APP_ID=$(echo "$APP" | jq -r '.id')
APP_SLUG=$(echo "$APP" | jq -r '.slug')
assert_not_empty "Phase153 app id"   "$APP_ID"
assert_not_empty "Phase153 app slug" "$APP_SLUG"

# Public app lookup (anonymous)
APP_BY_SLUG=$(curl -sSk "$BASE/api/v3/apps/$APP_SLUG")
SLUG_FROM_PUBLIC=$(echo "$APP_BY_SLUG" | jq -r '.slug')
assert_eq "Phase153 GET /apps/{slug} anon" "$APP_SLUG" "$SLUG_FROM_PUBLIC"
PEM_LEAK=$(echo "$APP_BY_SLUG" | jq -r '.pem // ""')
assert_eq "Phase153 public app no PEM leak" "" "$PEM_LEAK"

# Create an installation
INST=$(api "$BASE/api/v3/bleephub/apps/$APP_ID/installations" \
    -f target_type=User -f target_id=1 -f target_login=admin \
    -f 'permissions[issues]=write' -f 'permissions[checks]=write')
INST_ID=$(echo "$INST" | jq -r '.id')
assert_not_empty "Phase153 installation id" "$INST_ID"
SELECTION=$(echo "$INST" | jq -r '.repository_selection')
assert_eq "Phase153 installation default repository_selection" "all" "$SELECTION"
# HATEOAS url fields
ACCESS_URL=$(echo "$INST" | jq -r '.access_tokens_url')
case "$ACCESS_URL" in
    *"/api/v3/app/installations/$INST_ID/access_tokens"*) pass "Phase153 installation access_tokens_url" ;;
    *) fail "Phase153 access_tokens_url shape: $ACCESS_URL" ;;
esac

# Suspend / unsuspend (sim mgmt path)
SUSPEND_CODE=$(curl -sSk -X POST -H "Authorization: token $TOKEN" \
    "$BASE/api/v3/bleephub/installations/$INST_ID/suspend" -w "%{http_code}" -o /dev/null)
assert_eq "Phase153 suspend installation 204" "204" "$SUSPEND_CODE"
UNSUSP_CODE=$(curl -sSk -X POST -H "Authorization: token $TOKEN" \
    "$BASE/api/v3/bleephub/installations/$INST_ID/unsuspend" -w "%{http_code}" -o /dev/null)
assert_eq "Phase153 unsuspend installation 204" "204" "$UNSUSP_CODE"

# Installation lookup by user
USR_INST=$(curl -sSk -H "Authorization: token $TOKEN" "$BASE/api/v3/users/admin/installation")
USR_INST_ID=$(echo "$USR_INST" | jq -r '.id // 0')
assert_eq "Phase153 GET /users/{login}/installation id matches" "$INST_ID" "$USR_INST_ID"

# OAuth App create + Basic-auth on /applications/{client_id}/token
OA=$(api "$BASE/api/v3/bleephub/oauth-apps" -f name="OA Parity" -f description="Phase 153" \
    -f url="https://example.test" -f callback_url="https://example.test/cb")
OA_CID=$(echo "$OA" | jq -r '.client_id')
OA_CSEC=$(echo "$OA" | jq -r '.client_secret')
assert_not_empty "Phase153 oauth app client_id"     "$OA_CID"
assert_not_empty "Phase153 oauth app client_secret" "$OA_CSEC"

# Unknown token → 404
ACTOK_404=$(curl -sSk -X POST -u "$OA_CID:$OA_CSEC" \
    -H "Content-Type: application/json" \
    -d '{"access_token":"gho_does_not_exist"}' \
    "$BASE/api/v3/applications/$OA_CID/token" -w "%{http_code}" -o /dev/null)
assert_eq "Phase153 /applications/{client_id}/token unknown → 404" "404" "$ACTOK_404"

# Wrong client secret → 401
ACTOK_401=$(curl -sSk -X POST -u "$OA_CID:wrong-secret" \
    -H "Content-Type: application/json" \
    -d '{"access_token":"gho_x"}' \
    "$BASE/api/v3/applications/$OA_CID/token" -w "%{http_code}" -o /dev/null)
assert_eq "Phase153 /applications/{client_id}/token wrong secret → 401" "401" "$ACTOK_401"

log "Phase 153 parity probes complete"

# ============================================================
# Summary
# ============================================================
echo ""
echo "=============================="
echo "  gh CLI Test Results"
echo "=============================="
echo "  PASS: $PASS"
echo "  FAIL: $FAIL"
echo "=============================="

if [ "$FAIL" -gt 0 ]; then
    echo -e "Failures:$ERRORS"
    echo ""
    echo "=== last 80 lines of bleephub log (debug-level) for the failures ==="
    tail -80 /tmp/bleephub.log 2>/dev/null || true
    kill $BPH_PID 2>/dev/null || true
    exit 1
fi

log "All tests passed!"
kill $BPH_PID 2>/dev/null || true
exit 0
