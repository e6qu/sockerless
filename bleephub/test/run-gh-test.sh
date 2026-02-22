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
bleephub -addr :443 &
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
TOKEN="bph_0000000000000000000000000000000000000000"
BASE="https://localhost"

# We'll use gh api with full URLs + -H for auth, since gh auth login for
# custom GHES hostnames can be tricky. This tests the exact same REST/GraphQL
# endpoints that gh CLI uses.

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
# Test: Create repo
# ============================================================
log "Test: Create repo"
REPO=$(api "$BASE/api/v3/user/repos" -f name=gh-test-repo -f description="GH CLI test" -f private=false)
REPO_NAME=$(echo "$REPO" | jq -r '.name')
assert_eq "repo name" "gh-test-repo" "$REPO_NAME"
REPO_FULLNAME=$(echo "$REPO" | jq -r '.full_name')
assert_eq "repo full_name" "admin/gh-test-repo" "$REPO_FULLNAME"

# Verify permissions in response
PERMS_ADMIN=$(echo "$REPO" | jq -r '.permissions.admin')
assert_eq "repo permissions.admin" "true" "$PERMS_ADMIN"

# ============================================================
# Test: List repos
# ============================================================
log "Test: List repos"
REPOS=$(api "$BASE/api/v3/user/repos")
REPO_COUNT=$(echo "$REPOS" | jq 'length')
if [ "$REPO_COUNT" -ge 1 ]; then
    pass "list repos returns >= 1"
else
    fail "list repos returned $REPO_COUNT"
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
# Test: Create issue
# ============================================================
log "Test: Create issue"
ISSUE=$(api "$BASE/api/v3/repos/admin/gh-test-repo/issues" -f title="GH CLI issue" -f body="Testing via gh api")
ISSUE_NUM=$(echo "$ISSUE" | jq -r '.number')
assert_eq "issue number" "1" "$ISSUE_NUM"
ISSUE_STATE=$(echo "$ISSUE" | jq -r '.state')
assert_eq "issue state" "open" "$ISSUE_STATE"

# ============================================================
# Test: Get issue
# ============================================================
log "Test: Get issue"
ISSUE_GET=$(api "$BASE/api/v3/repos/admin/gh-test-repo/issues/1")
ISSUE_TITLE=$(echo "$ISSUE_GET" | jq -r '.title')
assert_eq "get issue title" "GH CLI issue" "$ISSUE_TITLE"

# ============================================================
# Test: List issues
# ============================================================
log "Test: List issues"
ISSUES=$(api "$BASE/api/v3/repos/admin/gh-test-repo/issues")
ISSUE_LIST_COUNT=$(echo "$ISSUES" | jq 'length')
if [ "$ISSUE_LIST_COUNT" -ge 1 ]; then
    pass "list issues returns >= 1"
else
    fail "list issues returned $ISSUE_LIST_COUNT"
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
    kill $BPH_PID 2>/dev/null || true
    exit 1
fi

log "All tests passed!"
kill $BPH_PID 2>/dev/null || true
exit 0
