#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-or-later
#
# Live-origin smoke test for the Sockerless deployment recipe: every service
# answers /health, every console redirects an unauthenticated browser to
# Shauth, Shauth's discovery document serves, every simulator data plane
# rejects an unauthenticated call and answers an authenticated one with the
# simulator's own seeded credentials, and `sockerless login --no-browser`
# prints a Shauth authorize URL that resolves to a real sign-in form.
#
# Run deploy/provision.sh first — this script assumes the full stack
# (including the "console" profile) is already up and provisioned.
set -euo pipefail

deploy_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
cd "$deploy_dir"
repo_root=$(cd .. && pwd)

for command in bash curl jq openssl go; do
  command -v "$command" >/dev/null 2>&1 || {
    echo "error: $command is required" >&2
    exit 1
  }
done

[[ -f .env ]] || {
  echo "error: deploy/.env not found — run deploy/provision.sh first" >&2
  exit 1
}
set -a
# shellcheck source=/dev/null
source .env
# shellcheck source=/dev/null
[[ -f .env.generated ]] && source .env.generated
set +a

# caddy issues every persistent origin's certificate from its own local
# certificate authority (deploy/Caddyfile's `tls internal`); provision.sh
# already extracted it. Export it once so every curl call below trusts it
# automatically, matching how a browser would need to trust the same
# authority once (see deploy/README.md "Security").
caddy_ca_file="$deploy_dir/.caddy-local-ca.crt"
[[ -s $caddy_ca_file ]] || {
  echo "error: $caddy_ca_file not found — run deploy/provision.sh first" >&2
  exit 1
}
export CURL_CA_BUNDLE="$caddy_ca_file"

failures=0
pass() { printf 'PASS  %s\n' "$1"; }
fail() {
  printf 'FAIL  %s\n' "$1" >&2
  failures=$((failures + 1))
}

# ---------------------------------------------------------------------------
# /health / /healthz on every service.
# ---------------------------------------------------------------------------
check_health() {
  local name=$1 url=$2 status
  status=$(curl --silent --output /dev/null --write-out '%{http_code}' "$url")
  if [[ $status == 200 ]]; then
    pass "$name health ($url)"
  else
    fail "$name health ($url): HTTP $status"
  fi
}
check_health Shauth "$SHAUTH_PUBLIC_URL/healthz"
check_health "Sockerless Admin" "$ADMIN_PUBLIC_URL/healthz"
check_health "AWS simulator" "$AWS_SIM_PUBLIC_URL/health"
check_health "Google Cloud simulator" "$GCP_SIM_PUBLIC_URL/health"
check_health "Microsoft Azure simulator (cloud)" "https://$AZURE_CLOUD_SIM_HOSTNAME:$CADDY_PORT/health"
check_health "Microsoft Azure simulator (console)" "$AZURE_CONSOLE_SIM_PUBLIC_URL/health"

# ---------------------------------------------------------------------------
# Shauth discovery document.
# ---------------------------------------------------------------------------
discovery=$(curl --silent --show-error --fail "$SHAUTH_PUBLIC_URL/.well-known/openid-configuration")
if jq -e '.issuer and .authorization_endpoint and .token_endpoint and .jwks_uri' <<<"$discovery" >/dev/null; then
  pass "Shauth OpenID Connect discovery document"
else
  fail "Shauth OpenID Connect discovery document missing required fields"
fi

# ---------------------------------------------------------------------------
# Each console's /ui/ redirects an unauthenticated browser to Shauth.
# ---------------------------------------------------------------------------
check_ui_redirect() {
  local name=$1 url=$2 headers status location
  headers=$(curl --silent --dump-header - --output /dev/null --max-redirs 0 "$url/ui/" || true)
  status=$(head -n1 <<<"$headers" | tr -d '\r')
  location=$(awk 'BEGIN{IGNORECASE=1} /^location:/{print $2}' <<<"$headers" | tr -d '\r')
  if [[ $status == *" 302"* || $status == *" 303"* ]] && [[ $location == *"$SHAUTH_HOSTNAME"* || $location == /auth/* ]]; then
    pass "$name /ui/ redirects unauthenticated to Shauth"
  else
    fail "$name /ui/ did not redirect to Shauth (status: $status, location: $location)"
  fi
}
check_ui_redirect "Sockerless Admin" "$ADMIN_PUBLIC_URL"
check_ui_redirect "AWS simulator" "$AWS_SIM_PUBLIC_URL"
check_ui_redirect "Google Cloud simulator" "$GCP_SIM_PUBLIC_URL"
check_ui_redirect "Microsoft Azure simulator (console)" "$AZURE_CONSOLE_SIM_PUBLIC_URL"

# ---------------------------------------------------------------------------
# AWS data plane: SigV4 required. Unauthenticated sts:GetCallerIdentity is
# refused; the same call signed with the simulator's seeded bootstrap
# credential (test/test, us-east-1 — the well-known coordinate every AWS
# SDK/CLI/Terraform test surface in this repository configures) answers.
# ---------------------------------------------------------------------------
aws_sim_host=${AWS_SIM_PUBLIC_URL#https://}
status=$(curl --silent --output /dev/null --write-out '%{http_code}' -X POST "https://$aws_sim_host/" \
  -H 'Content-Type: application/x-www-form-urlencoded' --data 'Action=GetCallerIdentity&Version=2011-06-15')
if [[ $status == 403 ]]; then
  pass "AWS data plane rejects unauthenticated sts:GetCallerIdentity (HTTP 403)"
else
  fail "AWS data plane did not reject unauthenticated sts:GetCallerIdentity (HTTP $status, expected 403)"
fi

hex_sha256() { printf '%s' "$1" | openssl dgst -sha256 | sed -E 's/^.*=[[:space:]]*//'; }
hmac_hex() { printf '%s' "$2" | openssl dgst -sha256 -mac HMAC -macopt "hexkey:$1" | sed -E 's/^.*=[[:space:]]*//'; }
aws_sigv4_get_caller_identity() {
  local host=$aws_sim_host service=sts region=us-east-1 body='Action=GetCallerIdentity&Version=2011-06-15'
  local amzdate datestamp payload_hash signed_headers canonical_request scope string_to_sign
  amzdate=$(date -u +%Y%m%dT%H%M%SZ)
  datestamp=$(date -u +%Y%m%d)
  payload_hash=$(hex_sha256 "$body")
  signed_headers="content-type;host;x-amz-content-sha256;x-amz-date"
  canonical_request=$(printf 'POST\n/\n\ncontent-type:application/x-www-form-urlencoded\nhost:%s\nx-amz-content-sha256:%s\nx-amz-date:%s\n\n%s\n%s' \
    "$host" "$payload_hash" "$amzdate" "$signed_headers" "$payload_hash")
  scope="$datestamp/$region/$service/aws4_request"
  string_to_sign=$(printf 'AWS4-HMAC-SHA256\n%s\n%s\n%s' "$amzdate" "$scope" "$(hex_sha256 "$canonical_request")")
  local k_date k_region k_service k_signing signature
  k_date=$(printf '%s' "$datestamp" | openssl dgst -sha256 -hmac "AWS4test" | sed -E 's/^.*=[[:space:]]*//')
  k_region=$(hmac_hex "$k_date" "$region")
  k_service=$(hmac_hex "$k_region" "$service")
  k_signing=$(hmac_hex "$k_service" "aws4_request")
  signature=$(hmac_hex "$k_signing" "$string_to_sign")
  curl --silent --show-error -X POST "https://$host/" \
    -H 'Content-Type: application/x-www-form-urlencoded' \
    -H "X-Amz-Date: $amzdate" \
    -H "X-Amz-Content-Sha256: $payload_hash" \
    -H "Authorization: AWS4-HMAC-SHA256 Credential=test/$scope, SignedHeaders=$signed_headers, Signature=$signature" \
    --data "$body"
}
aws_response=$(aws_sigv4_get_caller_identity)
if grep -q '<Arn>' <<<"$aws_response"; then
  pass "AWS data plane answers authenticated sts:GetCallerIdentity"
else
  fail "AWS data plane did not answer authenticated sts:GetCallerIdentity: $aws_response"
fi

# ---------------------------------------------------------------------------
# GCP data plane: OAuth2 bearer required. Unauthenticated read of the
# workforce pool provision.sh created is refused; the bootstrap token
# minter (exempt, like real Google's own token endpoint) mints a bearer that
# answers.
# ---------------------------------------------------------------------------
gcp_pool_url="$GCP_SIM_PUBLIC_URL/v1/locations/global/workforcePools/sockerless-console"
status=$(curl --silent --output /dev/null --write-out '%{http_code}' "$gcp_pool_url")
if [[ $status == 401 ]]; then
  pass "GCP data plane rejects unauthenticated workforcePools.get (HTTP 401)"
else
  fail "GCP data plane did not reject unauthenticated workforcePools.get (HTTP $status, expected 401)"
fi
gcp_token=$(curl --silent --show-error --fail -X POST "$GCP_SIM_PUBLIC_URL/token" \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  --data-urlencode 'grant_type=client_credentials' \
  --data-urlencode 'scope=https://www.googleapis.com/auth/cloud-platform' | jq -r '.access_token')
gcp_response=$(curl --silent --show-error --fail -H "Authorization: Bearer $gcp_token" "$gcp_pool_url")
if jq -e '.name == "locations/global/workforcePools/sockerless-console"' <<<"$gcp_response" >/dev/null; then
  pass "GCP data plane answers authenticated workforcePools.get"
else
  fail "GCP data plane did not answer authenticated workforcePools.get: $gcp_response"
fi

# ---------------------------------------------------------------------------
# Azure data plane: Azure Resource Manager bearer required. Unauthenticated
# resource-group listing is refused; the simulator's seeded bootstrap
# service principal (test-client-id/test-client-secret, the coordinate the
# simulator's own Terraform docs use) answers.
# ---------------------------------------------------------------------------
azure_cloud_base="https://$AZURE_CLOUD_SIM_HOSTNAME:$CADDY_PORT"
azure_subscription=00000000-0000-0000-0000-000000000001
azure_rg_url="$azure_cloud_base/subscriptions/$azure_subscription/resourceGroups?api-version=2021-04-01"
status=$(curl --silent --output /dev/null --write-out '%{http_code}' "$azure_rg_url")
if [[ $status == 401 ]]; then
  pass "Azure data plane rejects unauthenticated resourceGroups.list (HTTP 401)"
else
  fail "Azure data plane did not reject unauthenticated resourceGroups.list (HTTP $status, expected 401)"
fi
azure_bearer=$(curl --silent --show-error --fail -X POST "$azure_cloud_base/$AZURE_FEDERATION_TENANT/oauth2/v2.0/token" \
  --data-urlencode grant_type=client_credentials \
  --data-urlencode client_id=test-client-id \
  --data-urlencode client_secret=test-client-secret \
  --data-urlencode 'scope=https://management.azure.com/.default' | jq -r '.access_token')
azure_response=$(curl --silent --show-error --fail -H "Authorization: Bearer $azure_bearer" "$azure_rg_url")
if jq -e '.value | type == "array"' <<<"$azure_response" >/dev/null; then
  pass "Azure data plane answers authenticated resourceGroups.list"
else
  fail "Azure data plane did not answer authenticated resourceGroups.list: $azure_response"
fi

# ---------------------------------------------------------------------------
# `sockerless login --no-browser` prints a valid Shauth authorize URL; curl
# it and follow the redirect chain to Shauth's real login form.
# ---------------------------------------------------------------------------
work_dir=$(mktemp -d)
trap 'rm -rf "$work_dir"' EXIT

# The CLI's own OpenID Connect discovery call needs to trust caddy's local
# certificate authority the same way curl does above via CURL_CA_BUNDLE —
# but crypto/x509's SSL_CERT_FILE override is a Unix/Linux convention
# (root_unix.go) that Darwin's root-of-trust implementation (root_darwin.go)
# does not consult at all, so running the host-built binary directly trusts
# caddy's certificate on Linux but never on macOS. Building for linux and
# running it in a throwaway container on the compose network sidesteps that
# entirely — no code change to the read-only cmd/sockerless CLI needed, and
# it exercises exactly the SSL_CERT_FILE path a real Linux deployment host
# (or this repository's own Linux CI) uses natively.
GOWORK=off CGO_ENABLED=0 GOOS=linux GOARCH="$(go env GOARCH)" \
  go -C "$repo_root/cmd/sockerless" build -o "$work_dir/sockerless-linux" .
mkdir -p "$work_dir/cli-home"
cat >"$work_dir/cli-home/config.yaml" <<EOF
environments:
  smoke:
    backend: docker
    login:
      issuer: $SHAUTH_PUBLIC_URL
      client_id: sockerless-cli
    aws:
      region: us-east-1
      login:
        role_arn: arn:aws:iam::123456789012:role/cli-federation-role
        endpoint_url: $AWS_SIM_PUBLIC_URL
EOF
printf 'smoke\n' >"$work_dir/cli-home/active"

# The compose project name is fixed ("sockerless-deploy" in both
# compose.yaml and compose.build.yaml), so its default network name is
# deterministic — this container joins it to resolve every *.localtest.me
# hostname exactly as every other service does (deploy/README.md
# "Persistent origins").
login_log="$work_dir/login.log"
docker run --rm --network sockerless-deploy_default \
  -v "$work_dir/sockerless-linux:/sockerless:ro" \
  -v "$caddy_ca_file:/ca.crt:ro" \
  -v "$work_dir/cli-home:/cli-home" \
  -e SOCKERLESS_HOME=/cli-home -e SSL_CERT_FILE=/ca.crt \
  alpine:3.20 /sockerless login --no-browser --timeout 5s \
  >"$login_log" 2>&1 &
login_pid=$!

authorize_url=""
for _ in $(seq 1 20); do
  authorize_url=$(grep -Eo 'https?://[^[:space:]]+' "$login_log" | head -n1 || true)
  [[ -n $authorize_url ]] && break
  sleep 0.5
done
kill "$login_pid" 2>/dev/null || true
wait "$login_pid" 2>/dev/null || true
docker kill "$(docker ps -q --filter ancestor=alpine:3.20 --filter network=sockerless-deploy_default)" >/dev/null 2>&1 || true

if [[ -z $authorize_url ]]; then
  fail "sockerless login --no-browser printed no authorize URL ($(cat "$login_log"))"
else
  login_page=$(curl --silent --show-error --fail --location "$authorize_url")
  if grep -qi 'password\|sign in\|<form' <<<"$login_page"; then
    pass "sockerless login --no-browser authorize URL resolves to a login form"
  else
    fail "sockerless login --no-browser authorize URL did not resolve to a login form"
  fi
fi

echo
if ((failures > 0)); then
  echo "smoke: $failures check(s) failed" >&2
  exit 1
fi
echo "smoke: all checks passed"
