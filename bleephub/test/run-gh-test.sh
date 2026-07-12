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
# The admin token has no default — the binary fails loudly if this is unset.
export BLEEPHUB_ADMIN_TOKEN="bleephub-admin-token-00000000000000000000"
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
TOKEN="bleephub-admin-token-00000000000000000000"
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
# Wire git pushes/pulls through gh's credential helper so native verbs that
# shell out to git (gh repo clone, gh pr create from a working dir) authenticate.
gh auth setup-git --hostname "$HOST" >/dev/null 2>&1 || true

# `api` for endpoints `gh` doesn't expose as a high-level command
# (apps/{slug}, /applications/{cid}/token, suspend, etc.). For the
# happy-path repository/issue/pull-request/release surface, use real
# `gh repo create`, `gh issue create`, `gh pr create`, `gh release create`
# below.
api() {
    gh api -H "Authorization: token $TOKEN" -H "Accept: application/vnd.github+json" "$@"
}

# ============================================================
# Test: application programming interface root
# ============================================================
log "Test: application programming interface root"
ROOT=$(api "$BASE/api/v3/")
assert_contains "application programming interface root has current_user_url" "$ROOT" "current_user_url"

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
# Test: GitHub Classroom browser writes + official gh API reads
# ============================================================
log "Test: GitHub Classroom"
api --method POST "$BASE/api/v3/admin/organizations" -f login=gh-classroom -f admin=admin >/dev/null
api --method POST "$BASE/api/v3/orgs/gh-classroom/repos" -f name=starter -F auto_init=true >/dev/null
CLASSROOM=$(api --method POST "$BASE/classroom-data/classrooms" -f name="GH CLI Classroom" -f organization=gh-classroom)
CLASSROOM_ID=$(echo "$CLASSROOM" | jq -r '.id')
ASSIGNMENT=$(api --method POST "$BASE/classroom-data/classrooms/$CLASSROOM_ID/assignments" \
    --input <(printf '%s' '{"title":"Command line assignment","type":"individual","starter_code_repository":"gh-classroom/starter","invitations_enabled":true,"autograding_tests":[{"name":"README","command":"test -f README.md","points":10}]}'))
ASSIGNMENT_ID=$(echo "$ASSIGNMENT" | jq -r '.id')
CLASSROOMS=$(api "$BASE/api/v3/classrooms")
assert_eq "gh api lists browser-created Classroom" "GH CLI Classroom" "$(echo "$CLASSROOMS" | jq -r --argjson id "$CLASSROOM_ID" '.[] | select(.id == $id) | .name')"
ASSIGNMENT_GET=$(api "$BASE/api/v3/assignments/$ASSIGNMENT_ID")
assert_eq "gh api reads Classroom starter repository" "gh-classroom/starter" "$(echo "$ASSIGNMENT_GET" | jq -r '.starter_code_repository.full_name')"

# ============================================================
# Test: Fine-grained personal access token browser producer
# ============================================================
log "Test: fine-grained personal access tokens"
api --method POST "$BASE/api/v3/admin/organizations" -f login=gh-pat -f admin=admin >/dev/null
PAT_CREATED=$(api --method POST "$BASE/settings/personal-access-tokens" \
    --input <(printf '%s' '{"name":"Command line token","resource_owner":"gh-pat","repository_selection":"none","permissions":{"organization":{"members":"read"}},"reason":"gh CLI parity"}'))
PAT_SECRET=$(echo "$PAT_CREATED" | jq -r '.token')
assert_contains "gh api creates a fine-grained credential" "$PAT_SECRET" "github_pat_"
PAT_SETTINGS=$(api "$BASE/settings/personal-access-tokens")
assert_eq "gh api lists browser-created fine-grained token" "Command line token" "$(echo "$PAT_SETTINGS" | jq -r '.tokens[] | select(.name == "Command line token") | .name')"
if echo "$PAT_SETTINGS" | grep -qF "$PAT_SECRET"; then
    fail "fine-grained token settings list exposed the one-time credential"
else
    pass "fine-grained token credential is shown only once"
fi
PAT_VIEWER=$(gh api -H "Authorization: token $PAT_SECRET" "$BASE/api/v3/user")
assert_eq "fine-grained token authenticates through gh api" "admin" "$(echo "$PAT_VIEWER" | jq -r '.login')"
if api "$BASE/api/v3/orgs/gh-pat/personal-access-token-requests" >/dev/null 2>&1; then
    fail "classic personal access token called GitHub App-only organization token administration"
else
    pass "organization token administration rejects classic personal access tokens"
fi

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
# Test: View issue via real `gh issue view` (Representational State Transfer-backed, --json optional)
# ============================================================
log "Test: gh issue view"
# `gh issue view N --repo ...` uses the Representational State Transfer
# application programming interface directly; --json args go through GraphQL on
# real GitHub. This checks the Representational State Transfer-only path by not
# passing --json; gh prints a human-readable summary on success.
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
rm -rf /tmp/gh-test-pr-seed
mkdir -p /tmp/gh-test-pr-seed
(
    cd /tmp/gh-test-pr-seed
    git init -q
    printf '# gh-test-repo\n' > README.md
    git add README.md
    git commit -q -m "initial commit"
    git remote add origin https://localhost/admin/gh-test-repo.git
    git push -q origin HEAD:main
    git checkout -q -b feature
    printf 'feature\n' > feature.txt
    git add feature.txt
    git commit -q -m "feature commit"
    git push -q origin HEAD:feature
)
pass "seeded real PR refs"
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
# Test: organization lifecycle (via application programming interface)
# ============================================================
log "Test: Create organization"
ORG=$(api -X POST "$BASE/api/v3/admin/organizations" \
    -f login=gh-test-org \
    -f admin=admin \
    -f profile_name="Test Org")
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
# GitHub Apps + OAuth Apps parity tests
# ============================================================
log "GitHub Apps + OAuth Apps surface"

# Create a GitHub App with explicit permissions + events through the GitHub
# App Manifest flow.
MANIFEST=$(jq -nc '{
    name: "Parity App",
    description: "parity test",
    url: "https://example.test/app",
    redirect_url: "https://example.test/callback",
    default_permissions: {issues: "write", checks: "write"},
    default_events: ["push", "installation"]
}')
MANIFEST_HEADERS=$(curl -sSk -o /dev/null -D - -X POST \
    -H "Authorization: token $TOKEN" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    --data-urlencode "manifest=$MANIFEST" \
    "$BASE/settings/apps/new")
MANIFEST_LOCATION=$(printf '%s\n' "$MANIFEST_HEADERS" | awk 'tolower($1) == "location:" {print $2}' | tr -d '\r')
MANIFEST_CODE=$(printf '%s\n' "$MANIFEST_LOCATION" | sed -n 's/.*[?&]code=\([^&]*\).*/\1/p')
assert_not_empty "GitHub App manifest conversion code" "$MANIFEST_CODE"
APP=$(curl -sSk -X POST "$BASE/api/v3/app-manifests/$MANIFEST_CODE/conversions")
APP_ID=$(echo "$APP" | jq -r '.id')
APP_SLUG=$(echo "$APP" | jq -r '.slug')
assert_not_empty "app id"   "$APP_ID"
assert_not_empty "app slug" "$APP_SLUG"

# Public app lookup (anonymous)
APP_BY_SLUG=$(curl -sSk "$BASE/api/v3/apps/$APP_SLUG")
SLUG_FROM_PUBLIC=$(echo "$APP_BY_SLUG" | jq -r '.slug')
assert_eq "GET /apps/{slug} anon" "$APP_SLUG" "$SLUG_FROM_PUBLIC"
PEM_LEAK=$(echo "$APP_BY_SLUG" | jq -r '.pem // ""')
assert_eq "public app no PEM leak" "" "$PEM_LEAK"

# Create an installation through the signed-in GitHub App browser flow.
INST=$(curl -sSk -X POST \
    -H "Authorization: token $TOKEN" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    --data-urlencode target_login=admin \
    --data-urlencode repository_selection=all \
    "$BASE/apps/$APP_SLUG/installations/new")
INST_ID=$(echo "$INST" | jq -r '.id')
assert_not_empty "installation id" "$INST_ID"
SELECTION=$(echo "$INST" | jq -r '.repository_selection')
assert_eq "installation default repository_selection" "all" "$SELECTION"
# HATEOAS url fields
ACCESS_URL=$(echo "$INST" | jq -r '.access_tokens_url')
case "$ACCESS_URL" in
    *"/api/v3/app/installations/$INST_ID/access_tokens"*) pass "installation access_tokens_url" ;;
    *) fail "access_tokens_url shape: $ACCESS_URL" ;;
esac

# Suspend / unsuspend through the signed-in settings path.
SUSPEND_CODE=$(curl -sSk -X POST -H "Authorization: token $TOKEN" \
    "$BASE/settings/installations/$INST_ID/suspend" -w "%{http_code}" -o /dev/null)
assert_eq "suspend installation 204" "204" "$SUSPEND_CODE"
UNSUSP_CODE=$(curl -sSk -X POST -H "Authorization: token $TOKEN" \
    "$BASE/settings/installations/$INST_ID/unsuspend" -w "%{http_code}" -o /dev/null)
assert_eq "unsuspend installation 204" "204" "$UNSUSP_CODE"

# Installation lookup by user
USR_INST=$(curl -sSk -H "Authorization: token $TOKEN" "$BASE/api/v3/users/admin/installation")
USR_INST_ID=$(echo "$USR_INST" | jq -r '.id // 0')
assert_eq "GET /users/{login}/installation id matches" "$INST_ID" "$USR_INST_ID"

# OAuth App create + Basic-auth on /applications/{client_id}/token
OA=$(api "$BASE/settings/oauth-apps/new" -f name="OA Parity" -f description="parity" \
    -f url="https://example.test" -f callback_url="https://example.test/cb")
OA_CID=$(echo "$OA" | jq -r '.client_id')
OA_CSEC=$(echo "$OA" | jq -r '.client_secret')
assert_not_empty "oauth app client_id"     "$OA_CID"
assert_not_empty "oauth app client_secret" "$OA_CSEC"

# Unknown token → 404
ACTOK_404=$(curl -sSk -X POST -u "$OA_CID:$OA_CSEC" \
    -H "Content-Type: application/json" \
    -d '{"access_token":"gho_does_not_exist"}' \
    "$BASE/api/v3/applications/$OA_CID/token" -w "%{http_code}" -o /dev/null)
assert_eq "/applications/{client_id}/token unknown → 404" "404" "$ACTOK_404"

# Wrong client secret → 401
ACTOK_401=$(curl -sSk -X POST -u "$OA_CID:wrong-secret" \
    -H "Content-Type: application/json" \
    -d '{"access_token":"gho_x"}' \
    "$BASE/api/v3/applications/$OA_CID/token" -w "%{http_code}" -o /dev/null)
assert_eq "/applications/{client_id}/token wrong secret → 401" "401" "$ACTOK_401"

# Marketplace listing production and buyer flow. OAuth Apps authenticate the
# publisher REST API with Basic client credentials; GitHub Apps use a JSON Web
# Token, which the official go-github suite exercises separately.
MARKETPLACE_SLUG="oauth-parity-marketplace"
MARKETPLACE_DRAFT=$(jq -nc --arg slug "$MARKETPLACE_SLUG" '{slug:$slug,name:"OAuth Parity Marketplace",description:"gh CLI Marketplace parity",full_description:"Real publisher and buyer workflow",installation_url:"https://example.test/install",webhook_url:"https://localhost/health",webhook_secret:"cli-marketplace",webhook_content_type:"json",webhook_active:true,published:false}')
MARKETPLACE_STATUS=$(curl -sSk -X PUT -H "Authorization: token $TOKEN" -H "Content-Type: application/json" -d "$MARKETPLACE_DRAFT" "$BASE/settings/oauth-apps/$OA_CID/marketplace" -w "%{http_code}" -o /tmp/marketplace-listing.json)
assert_eq "create Marketplace draft listing" "201" "$MARKETPLACE_STATUS"
MARKETPLACE_PLAN=$(curl -sSk -X POST -H "Authorization: token $TOKEN" -H "Content-Type: application/json" -d '{"name":"CLI Team","description":"Official gh API plan","price_model":"FLAT_RATE","monthly_price_in_cents":1800,"yearly_price_in_cents":18000,"state":"published"}' "$BASE/settings/oauth-apps/$OA_CID/marketplace/plans")
MARKETPLACE_PLAN_ID=$(echo "$MARKETPLACE_PLAN" | jq -r '.id')
assert_not_empty "create Marketplace pricing plan" "$MARKETPLACE_PLAN_ID"
MARKETPLACE_PUBLISHED=$(echo "$MARKETPLACE_DRAFT" | jq '.published=true')
MARKETPLACE_STATUS=$(curl -sSk -X PUT -H "Authorization: token $TOKEN" -H "Content-Type: application/json" -d "$MARKETPLACE_PUBLISHED" "$BASE/settings/oauth-apps/$OA_CID/marketplace" -w "%{http_code}" -o /dev/null)
assert_eq "publish Marketplace listing" "200" "$MARKETPLACE_STATUS"
MARKETPLACE_PURCHASE=$(curl -sSk -X POST -H "Authorization: token $TOKEN" -H "Content-Type: application/json" -d "{\"plan_id\":$MARKETPLACE_PLAN_ID,\"billing_cycle\":\"monthly\"}" "$BASE/ui-data/marketplace/listings/$MARKETPLACE_SLUG/purchase")
MARKETPLACE_INSTALLED=$(echo "$MARKETPLACE_PURCHASE" | jq -r '.marketplace_purchase.is_installed')
assert_eq "OAuth App Marketplace purchase uses installation URL without GitHub App installation" "false" "$MARKETPLACE_INSTALLED"
MARKETPLACE_PLAN_NAME=$(curl -sSk -u "$OA_CID:$OA_CSEC" "$BASE/api/v3/marketplace_listing/plans" | jq -r '.[0].name')
assert_eq "gh api-compatible Marketplace publisher plan list" "CLI Team" "$MARKETPLACE_PLAN_NAME"
MARKETPLACE_ACCOUNT=$(curl -sSk -u "$OA_CID:$OA_CSEC" "$BASE/api/v3/marketplace_listing/accounts/1" | jq -r '.login')
assert_eq "gh api-compatible Marketplace publisher account lookup" "admin" "$MARKETPLACE_ACCOUNT"

log "Apps parity probes complete"

# ============================================================
# PR-conversation parity — review threads, ProjectV2,
# edit history, minimization, locking, PR.milestone.
# Each block here exercises the surface added.
# ============================================================
log "PR-conversation parity probes…"

PR_REPO="admin/gh-test-repo"

# --- PR.comments — gh pr comment + gh pr view --json comments ---
if gh pr comment 2 --repo "$PR_REPO" --body "first review comment" >/dev/null 2>&1; then
    pass "gh pr comment exited 0"
else
    fail "gh pr comment exited non-zero"
fi
PR_COMMENTS=$(gh pr view 2 --repo "$PR_REPO" --json comments 2>/dev/null || echo '{}')
PR_COMMENT_COUNT=$(echo "$PR_COMMENTS" | jq '.comments | length')
if [ "$PR_COMMENT_COUNT" -ge 1 ]; then
    pass "PR.comments includes the new comment"
else
    fail "PR.comments empty after gh pr comment ($PR_COMMENTS)"
fi
PR_COMMENT_BODY=$(echo "$PR_COMMENTS" | jq -r '.comments[0].body')
assert_eq "PR.comments[0].body" "first review comment" "$PR_COMMENT_BODY"

# --- Comment edit history — PATCH a comment and verify lastEditedAt + body ---
PR_COMMENT_ID=$(api "$BASE/api/v3/repos/$PR_REPO/issues/2/comments" | jq -r '.[0].id')
if [ -n "$PR_COMMENT_ID" ] && [ "$PR_COMMENT_ID" != "null" ]; then
    EDITED=$(api -X PATCH "$BASE/api/v3/repos/$PR_REPO/issues/comments/$PR_COMMENT_ID" -f body="edited review comment")
    EDITED_BODY=$(echo "$EDITED" | jq -r '.body')
    assert_eq "edited comment body" "edited review comment" "$EDITED_BODY"
    # GraphQL view should report includesCreatedEdit=true now.
    EDIT_FLAG=$(gh pr view 2 --repo "$PR_REPO" --json comments \
        | jq -r '.comments[0].includesCreatedEdit // empty')
    if [ "$EDIT_FLAG" = "true" ]; then
        pass "comments[0].includesCreatedEdit after PATCH"
    else
        fail "includesCreatedEdit not flipped after PATCH (got $EDIT_FLAG)"
    fi
else
    fail "could not resolve PR comment id for edit test"
fi

# --- Minimization — direct GraphQL minimizeComment ---
COMMENT_NODE_ID=$(echo "$PR_COMMENTS" | jq -r '.comments[0].id')
if [ -n "$COMMENT_NODE_ID" ] && [ "$COMMENT_NODE_ID" != "null" ]; then
    MIN_RESP=$(curl -sSk -X POST -H "Authorization: bearer $TOKEN" -H "Content-Type: application/json" \
        -d "{\"query\":\"mutation { minimizeComment(input: {subjectId: \\\"$COMMENT_NODE_ID\\\", classifier: OFF_TOPIC}) { minimizedComment { id isMinimized minimizedReason } } }\"}" \
        "$BASE/api/graphql")
    IS_MIN=$(echo "$MIN_RESP" | jq -r '.data.minimizeComment.minimizedComment.isMinimized')
    MIN_REASON=$(echo "$MIN_RESP" | jq -r '.data.minimizeComment.minimizedComment.minimizedReason')
    assert_eq "minimizeComment isMinimized=true" "true" "$IS_MIN"
    assert_eq "minimizeComment minimizedReason" "OFF_TOPIC" "$MIN_REASON"
fi

# --- Locking — REST PUT /lock then attempt a new comment → expect 403 ---
LOCK_CODE=$(curl -sSk -X PUT -H "Authorization: token $TOKEN" -H "Content-Type: application/json" \
    -d '{"lock_reason":"too heated"}' \
    "$BASE/api/v3/repos/$PR_REPO/issues/2/lock" -w "%{http_code}" -o /dev/null)
assert_eq "lock PR 204" "204" "$LOCK_CODE"
POST_COMMENT_LOCKED=$(curl -sSk -X POST -H "Authorization: token $TOKEN" -H "Content-Type: application/json" \
    -d '{"body":"should be rejected"}' \
    "$BASE/api/v3/repos/$PR_REPO/issues/2/comments" -w "%{http_code}" -o /dev/null)
assert_eq "comment on locked PR 403" "403" "$POST_COMMENT_LOCKED"
UNLOCK_CODE=$(curl -sSk -X DELETE -H "Authorization: token $TOKEN" \
    "$BASE/api/v3/repos/$PR_REPO/issues/2/lock" -w "%{http_code}" -o /dev/null)
assert_eq "unlock PR 204" "204" "$UNLOCK_CODE"

# --- ProjectV2 — createProjectV2 + createProjectV2Field + addProjectV2ItemById + updateProjectV2ItemFieldValue ---
ADMIN_NODE_ID=$(curl -sSk -X POST -H "Authorization: bearer $TOKEN" -H "Content-Type: application/json" \
    -d '{"query":"{ viewer { id } }"}' "$BASE/api/graphql" | jq -r '.data.viewer.id')
if [ -n "$ADMIN_NODE_ID" ] && [ "$ADMIN_NODE_ID" != "null" ]; then
    CREATE_PROJ=$(curl -sSk -X POST -H "Authorization: bearer $TOKEN" -H "Content-Type: application/json" \
        -d "{\"query\":\"mutation { createProjectV2(input: {ownerId: \\\"$ADMIN_NODE_ID\\\", title: \\\"Review Board\\\"}) { projectV2 { id title number } } }\"}" \
        "$BASE/api/graphql")
    PROJ_NODE_ID=$(echo "$CREATE_PROJ" | jq -r '.data.createProjectV2.projectV2.id')
    PROJ_TITLE=$(echo "$CREATE_PROJ" | jq -r '.data.createProjectV2.projectV2.title')
    assert_not_empty "createProjectV2 id" "$PROJ_NODE_ID"
    assert_eq "createProjectV2 title" "Review Board" "$PROJ_TITLE"

    # Add a field with single-select options.
    CREATE_FIELD=$(curl -sSk -X POST -H "Authorization: bearer $TOKEN" -H "Content-Type: application/json" \
        -d "{\"query\":\"mutation { createProjectV2Field(input: {projectId: \\\"$PROJ_NODE_ID\\\", dataType: SINGLE_SELECT, name: \\\"Status\\\", singleSelectOptions: [{name: \\\"Todo\\\"}, {name: \\\"Done\\\"}]}) { projectV2Field { id name dataType } } }\"}" \
        "$BASE/api/graphql")
    FIELD_NODE_ID=$(echo "$CREATE_FIELD" | jq -r '.data.createProjectV2Field.projectV2Field.id')
    FIELD_NAME=$(echo "$CREATE_FIELD" | jq -r '.data.createProjectV2Field.projectV2Field.name')
    assert_not_empty "createProjectV2Field id" "$FIELD_NODE_ID"
    assert_eq "createProjectV2Field name" "Status" "$FIELD_NAME"

    # Add the seeded issue as a project item.
    ISSUE_NODE_ID=$(curl -sSk -X POST -H "Authorization: bearer $TOKEN" -H "Content-Type: application/json" \
        -d "{\"query\":\"{ repository(owner: \\\"admin\\\", name: \\\"gh-test-repo\\\") { issue(number: 1) { id } } }\"}" \
        "$BASE/api/graphql" | jq -r '.data.repository.issue.id')
    if [ -n "$ISSUE_NODE_ID" ] && [ "$ISSUE_NODE_ID" != "null" ]; then
        ADD_ITEM=$(curl -sSk -X POST -H "Authorization: bearer $TOKEN" -H "Content-Type: application/json" \
            -d "{\"query\":\"mutation { addProjectV2ItemById(input: {projectId: \\\"$PROJ_NODE_ID\\\", contentId: \\\"$ISSUE_NODE_ID\\\"}) { item { id } } }\"}" \
            "$BASE/api/graphql")
        ITEM_NODE_ID=$(echo "$ADD_ITEM" | jq -r '.data.addProjectV2ItemById.item.id')
        assert_not_empty "addProjectV2ItemById id" "$ITEM_NODE_ID"

        # Verify Issue.projectItems now returns the item via gh issue view --json projectItems
        # (gh CLI shells the GraphQL query for us).
        if gh issue view 1 --repo "$PR_REPO" --json projectItems >/tmp/bleephub-project-items.json 2>/dev/null; then
            ITEMS_LEN=$(jq '.projectItems | length' /tmp/bleephub-project-items.json)
            if [ "$ITEMS_LEN" -ge 1 ]; then
                pass "Issue.projectItems has the added item"
            else
                fail "Issue.projectItems empty after addItem"
            fi
        else
            fail "gh issue view --json projectItems failed"
        fi
    else
        fail "could not resolve issue node id"
    fi
fi

log "PR-conversation parity probes complete"

# ============================================================
# Native gh verb coverage — repo clone, the pr
# create→view→list→review→merge chain, the release lifecycle,
# issue label/close/reopen verbs, run view of a push-triggered
# run, and workflow run. All real gh verbs, no `gh api`.
# NOTE on jq: container jq is 1.6 which float-rounds int64 ids —
# always extract ids with gh's built-in --jq (gojq, int64-safe).
# ============================================================
log "Native gh verb coverage…"

ORIG_DIR=$(pwd)
NV_REPO="admin/gh-native-repo"

if gh repo create gh-native-repo --public >/dev/null 2>&1; then
    pass "gh repo create (native coverage repo)"
else
    fail "gh repo create gh-native-repo"
fi

# --- gh repo clone — sends the GraphQL RepositoryInfo query
# (hasWikiEnabled + parent) before the git clone ---
cd /tmp
rm -rf gh-native-clone
if gh repo clone "$NV_REPO" gh-native-clone >/dev/null 2>&1; then
    pass "gh repo clone"
else
    fail "gh repo clone"
fi
cd gh-native-clone

# Seed main with a push-triggered workflow (drives `gh run view` +
# `gh workflow run` below).
git checkout -q -b main 2>/dev/null || git checkout -q main
mkdir -p .github/workflows
cat > .github/workflows/ci.yml <<'YAML'
name: ci
on: [push, workflow_dispatch]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo hi
YAML
echo "native coverage" > README.md
git add .
git commit -q -m "init with workflow"
if git push -q origin main >/dev/null 2>&1; then
    pass "git push main (smart HTTP via gh credential helper)"
else
    fail "git push main"
fi

# --- gh run view of the push-triggered run ---
# The push above triggers the ci workflow; the run's workflow_id must
# resolve to the real workflow file or gh run view 404s.
sleep 2
RUN_ID=$(gh run list -R "$NV_REPO" --limit 1 --json databaseId --jq '.[0].databaseId' 2>/dev/null || echo "")
assert_not_empty "gh run list returns the push-triggered run" "$RUN_ID"
if [ -n "$RUN_ID" ]; then
    if gh run view "$RUN_ID" -R "$NV_REPO" >/dev/null 2>&1; then
        pass "gh run view of push-triggered run"
    else
        fail "gh run view $RUN_ID (workflow_id unresolvable?)"
    fi
fi

# --- gh workflow run (feature-detects via GET /meta first) ---
if gh workflow run ci.yml --ref main -R "$NV_REPO" >/dev/null 2>&1; then
    pass "gh workflow run"
else
    fail "gh workflow run ci.yml"
fi

# --- pr create→view→list→review→merge chain ---
git checkout -q -b feature
echo "feature work" >> README.md
git add README.md
git commit -q -m "feature commit"
git push -q origin feature >/dev/null 2>&1 || true

if gh pr create -R "$NV_REPO" --title "native pr" --body "native chain" --head feature --base main >/dev/null 2>&1; then
    pass "gh pr create"
else
    fail "gh pr create"
fi

NATIVE_PR=$(gh pr list -R "$NV_REPO" --json number --jq '.[0].number' 2>/dev/null || echo "")
assert_not_empty "gh pr list returns the new PR" "$NATIVE_PR"

if [ -n "$NATIVE_PR" ]; then
    if gh pr view "$NATIVE_PR" -R "$NV_REPO" >/dev/null 2>&1; then
        pass "gh pr view"
    else
        fail "gh pr view $NATIVE_PR"
    fi
    PR_TITLE=$(gh pr view "$NATIVE_PR" -R "$NV_REPO" --json title --jq .title 2>/dev/null || echo "")
    assert_eq "gh pr view --json title" "native pr" "$PR_TITLE"

    if gh pr review "$NATIVE_PR" -R "$NV_REPO" --approve --body "native lgtm" >/dev/null 2>&1; then
        pass "gh pr review --approve"
    else
        fail "gh pr review --approve"
    fi
    REVIEW_DECISION=$(gh pr view "$NATIVE_PR" -R "$NV_REPO" --json reviewDecision --jq .reviewDecision 2>/dev/null || echo "")
    assert_eq "reviewDecision after approve" "APPROVED" "$REVIEW_DECISION"

    if gh pr merge "$NATIVE_PR" -R "$NV_REPO" --merge >/dev/null 2>&1; then
        pass "gh pr merge"
    else
        fail "gh pr merge"
    fi
    PR_STATE=$(gh pr view "$NATIVE_PR" -R "$NV_REPO" --json state --jq .state 2>/dev/null || echo "")
    assert_eq "PR state after merge" "MERGED" "$PR_STATE"
fi

# --- release create→list→view→delete lifecycle ---
if gh release create v1.0.0 --notes "native release" -R "$NV_REPO" >/dev/null 2>&1; then
    pass "gh release create"
else
    fail "gh release create"
fi
REL_TAGS=$(gh release list -R "$NV_REPO" --json tagName --jq '.[].tagName' 2>/dev/null || echo "")
assert_contains "gh release list shows v1.0.0" "$REL_TAGS" "v1.0.0"
if gh release view v1.0.0 -R "$NV_REPO" >/dev/null 2>&1; then
    pass "gh release view"
else
    fail "gh release view v1.0.0"
fi
if gh release delete v1.0.0 -R "$NV_REPO" --yes >/dev/null 2>&1; then
    pass "gh release delete"
else
    fail "gh release delete v1.0.0"
fi
REL_COUNT=$(gh release list -R "$NV_REPO" --json tagName --jq 'length' 2>/dev/null || echo "-1")
assert_eq "release gone after delete" "0" "$REL_COUNT"

# --- CodeQL Action database upload protocol + gh API lifecycle ---
# The producer sends a finalized relocatable ZIP to the uploads host without
# an /api/v3 prefix; gh then consumes GitHub's public database REST resources.
CODEQL_COMMIT=$(api "$BASE/api/v3/repos/$NV_REPO/commits/main" --jq .sha 2>/dev/null || echo "")
assert_not_empty "CodeQL database source commit" "$CODEQL_COMMIT"
rm -rf /tmp/codeql-go-database /tmp/codeql-go-database.zip
mkdir -p /tmp/codeql-go-database/db-go/default/cache/pages
cat > /tmp/codeql-go-database/codeql-database.yml <<'YAML'
primaryLanguage: go
YAML
echo "real gh harness CodeQL dataset" > /tmp/codeql-go-database/db-go/default/cache/pages/0
(cd /tmp && zip -qr codeql-go-database.zip codeql-go-database)
if gh api --method POST \
    -H "Authorization: token $TOKEN" \
    -H "Content-Type: application/zip" \
    --input /tmp/codeql-go-database.zip \
    "$BASE/repos/$NV_REPO/code-scanning/codeql/databases/go?name=go-database&commit_oid=$CODEQL_COMMIT" >/dev/null 2>&1; then
    pass "CodeQL Action database upload protocol"
else
    fail "CodeQL Action database upload protocol"
fi
CODEQL_DATABASES=$(api "$BASE/api/v3/repos/$NV_REPO/code-scanning/codeql/databases")
assert_eq "gh api lists CodeQL database" "go-database" "$(echo "$CODEQL_DATABASES" | jq -r '.[0].name')"
CODEQL_DATABASE=$(api "$BASE/api/v3/repos/$NV_REPO/code-scanning/codeql/databases/go")
assert_eq "gh api gets CodeQL database language" "go" "$(echo "$CODEQL_DATABASE" | jq -r '.language')"
if api --method DELETE "$BASE/api/v3/repos/$NV_REPO/code-scanning/codeql/databases/go" >/dev/null 2>&1; then
    pass "gh api deletes CodeQL database"
else
    fail "gh api deletes CodeQL database"
fi
CODEQL_DATABASE_COUNT=$(api "$BASE/api/v3/repos/$NV_REPO/code-scanning/codeql/databases" --jq 'length' 2>/dev/null || echo "-1")
assert_eq "CodeQL database absent after delete" "0" "$CODEQL_DATABASE_COUNT"

# --- issue verbs: create with label, list --label, close, reopen ---
gh label create bug --color d73a4a -R "$NV_REPO" >/dev/null 2>&1 || true
if gh issue create -R "$NV_REPO" --title "native labeled issue" --body "native" --label bug >/dev/null 2>&1; then
    pass "gh issue create --label"
else
    fail "gh issue create --label"
fi
gh issue create -R "$NV_REPO" --title "native plain issue" --body "native" >/dev/null 2>&1 || true

# --label routes through GraphQL search(type: ISSUE), gated on GET /meta.
LABELED_COUNT=$(gh issue list -R "$NV_REPO" --label bug --json number --jq 'length' 2>/dev/null || echo "-1")
assert_eq "gh issue list --label bug count" "1" "$LABELED_COUNT"
LABELED_NUM=$(gh issue list -R "$NV_REPO" --label bug --json number --jq '.[0].number' 2>/dev/null || echo "")
assert_not_empty "labeled issue number" "$LABELED_NUM"

if [ -n "$LABELED_NUM" ]; then
    if gh issue close "$LABELED_NUM" -R "$NV_REPO" >/dev/null 2>&1; then
        pass "gh issue close"
    else
        fail "gh issue close"
    fi
    ISSUE_STATE=$(gh issue view "$LABELED_NUM" -R "$NV_REPO" --json state --jq .state 2>/dev/null || echo "")
    assert_eq "issue state after gh issue close" "CLOSED" "$ISSUE_STATE"

    if gh issue reopen "$LABELED_NUM" -R "$NV_REPO" >/dev/null 2>&1; then
        pass "gh issue reopen"
    else
        fail "gh issue reopen"
    fi
    ISSUE_STATE=$(gh issue view "$LABELED_NUM" -R "$NV_REPO" --json state --jq .state 2>/dev/null || echo "")
    assert_eq "issue state after gh issue reopen" "OPEN" "$ISSUE_STATE"
fi

cd "$ORIG_DIR"
log "Native gh verb coverage complete"

# ============================================================
# Organization surface: `gh org list` (native verb) plus the membership,
# team, and global-list endpoints via `gh api`.
# ============================================================
log "Org surface…"

if api -X POST "$BASE/api/v3/admin/organizations" \
    -f login=gh-native-org -f admin=admin >/dev/null 2>&1; then
    pass "create organization via GitHub Enterprise Server admin application programming interface"
else
    fail "POST /admin/organizations gh-native-org"
fi

ORG_LIST=$(gh org list 2>/dev/null || echo "")
assert_contains "gh org list shows the org" "$ORG_LIST" "gh-native-org"

GLOBAL_ORGS=$(api "$BASE/api/v3/organizations" --jq '.[].login' 2>/dev/null || echo "")
assert_contains "GET /organizations shows the org" "$GLOBAL_ORGS" "gh-native-org"

MEMBER_STATE=$(api "$BASE/api/v3/user/memberships/orgs/gh-native-org" --jq .state 2>/dev/null || echo "")
assert_eq "creator membership state" "active" "$MEMBER_STATE"

if api -X GET "$BASE/api/v3/orgs/gh-native-org/members/admin" >/dev/null 2>&1; then
    pass "member check (204)"
else
    fail "GET /orgs/gh-native-org/members/admin"
fi

if api -X POST "$BASE/api/v3/orgs/gh-native-org/teams" -f name=crew -f permission=push >/dev/null 2>&1; then
    pass "create team via gh api"
else
    fail "POST /orgs/gh-native-org/teams"
fi
TEAM_ROLE=$(api -X PUT "$BASE/api/v3/orgs/gh-native-org/teams/crew/memberships/admin" -f role=maintainer --jq .role 2>/dev/null || echo "")
assert_eq "team membership role" "maintainer" "$TEAM_ROLE"

log "Org surface complete"

# ============================================================
# Actions: secrets, variables, workflow enable/disable, checks.
# gh secret set exercises the REAL sealed-box contract (public-key
# fetch + libsodium crypto_box_seal client-side); gh variable set
# exercises the POST→409→PATCH fallback.
# ============================================================
log "Actions secrets/variables/workflow-state…"

if gh secret set DEPLOY_TOKEN -R "$NV_REPO" --body "s3cret-value" >/dev/null 2>&1; then
    pass "gh secret set (sealed box round-trip)"
else
    fail "gh secret set DEPLOY_TOKEN"
fi
SECRET_LIST=$(gh secret list -R "$NV_REPO" 2>/dev/null || echo "")
assert_contains "gh secret list shows the secret" "$SECRET_LIST" "DEPLOY_TOKEN"
# Updating re-encrypts with the same public key (204 path).
if gh secret set DEPLOY_TOKEN -R "$NV_REPO" --body "rotated-value" >/dev/null 2>&1; then
    pass "gh secret set update"
else
    fail "gh secret set update"
fi
if gh secret delete DEPLOY_TOKEN -R "$NV_REPO" >/dev/null 2>&1; then
    pass "gh secret delete"
else
    fail "gh secret delete"
fi

if gh variable set REGION -R "$NV_REPO" --body "eu-west-1" >/dev/null 2>&1; then
    pass "gh variable set"
else
    fail "gh variable set REGION"
fi
VAR_VALUE=$(gh variable get REGION -R "$NV_REPO" 2>/dev/null || echo "")
assert_eq "gh variable get" "eu-west-1" "$VAR_VALUE"
# Second set hits the POST→409→PATCH update path in gh.
if gh variable set REGION -R "$NV_REPO" --body "us-east-1" >/dev/null 2>&1; then
    pass "gh variable set (update via 409→PATCH)"
else
    fail "gh variable set update"
fi
VAR_VALUE=$(gh variable get REGION -R "$NV_REPO" 2>/dev/null || echo "")
assert_eq "gh variable get after update" "us-east-1" "$VAR_VALUE"
VAR_LIST=$(gh variable list -R "$NV_REPO" 2>/dev/null || echo "")
assert_contains "gh variable list" "$VAR_LIST" "REGION"
if gh variable delete REGION -R "$NV_REPO" >/dev/null 2>&1; then
    pass "gh variable delete"
else
    fail "gh variable delete"
fi

# Workflow enable/disable: disabled workflows reject dispatch.
if gh workflow disable ci.yml -R "$NV_REPO" >/dev/null 2>&1; then
    pass "gh workflow disable"
else
    fail "gh workflow disable ci.yml"
fi
WF_STATE=$(api "$BASE/api/v3/repos/$NV_REPO/actions/workflows/ci.yml" --jq .state 2>/dev/null || echo "")
assert_eq "workflow state after disable" "disabled_manually" "$WF_STATE"
if gh workflow run ci.yml --ref main -R "$NV_REPO" >/dev/null 2>&1; then
    fail "dispatch of a disabled workflow must be rejected"
else
    pass "dispatch rejected while disabled"
fi
if gh workflow enable ci.yml -R "$NV_REPO" >/dev/null 2>&1; then
    pass "gh workflow enable"
else
    fail "gh workflow enable ci.yml"
fi
WF_STATE=$(api "$BASE/api/v3/repos/$NV_REPO/actions/workflows/ci.yml" --jq .state 2>/dev/null || echo "")
assert_eq "workflow state after enable" "active" "$WF_STATE"

# Checks layer: the earlier pushes triggered ci runs, which mirror to
# check runs on the pushed commit (github-actions app).
HEAD_SHA=$(git -C /tmp/gh-native-clone rev-parse HEAD 2>/dev/null || echo "")
if [ -n "$HEAD_SHA" ]; then
    CHECKS_COUNT=$(api "$BASE/api/v3/repos/$NV_REPO/commits/$HEAD_SHA/check-runs" --jq .total_count 2>/dev/null || echo "0")
    if [ "$CHECKS_COUNT" -ge 1 ] 2>/dev/null; then
        pass "check runs mirror workflow jobs ($CHECKS_COUNT on $HEAD_SHA)"
    else
        fail "no check runs on pushed commit $HEAD_SHA"
    fi
fi

log "Actions secrets/variables/workflow-state complete"

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
