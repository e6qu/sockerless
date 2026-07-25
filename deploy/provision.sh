#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-or-later
#
# Idempotent provisioning for the Sockerless deployment recipe: registers
# every console + Admin as a Shauth OpenID Connect client (via Shauth's own
# bootstrap-apps mechanism, plus the CLI's public Hydra client, which is
# deliberately not a managed app — see the comment near
# register_cli_client), then provisions the federation resources each
# console's credential-minting and data-plane pages federate through, using
# only the clouds' own real APIs — exactly what an administrator would run
# by hand against a real AWS/GCP/Azure account, pointed at the simulators'
# coordinates instead. See deploy/README.md for the full runbook.
#
# Usage:
#   deploy/provision.sh                 # boot (if needed) + provision everything
#   DEPLOY_COMPOSE_FILE=compose.build.yaml deploy/provision.sh
set -euo pipefail

deploy_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
cd "$deploy_dir"

for command in bash curl jq openssl docker; do
  command -v "$command" >/dev/null 2>&1 || {
    echo "error: $command is required" >&2
    exit 1
  }
done

[[ -f .env ]] || {
  echo "error: deploy/.env not found — copy deploy/.env.example to deploy/.env and fill it in" >&2
  exit 1
}

compose_file=${DEPLOY_COMPOSE_FILE:-compose.yaml}
[[ -f $compose_file ]] || {
  echo "error: $compose_file not found in deploy/" >&2
  exit 1
}
touch .env.generated

compose() {
  docker compose -f "$compose_file" --env-file .env --env-file .env.generated "$@"
}

log() { printf '▸ %s\n' "$1" >&2; }

# set -a exports every subsequently sourced variable, so compose's own
# --env-file handling and this script's use of the same coordinates agree
# on one value each — never two parallel copies drifting apart.
set -a
# shellcheck source=/dev/null
source .env
set +a

for name in SHAUTH_PUBLIC_URL ADMIN_PUBLIC_URL AWS_SIM_PUBLIC_URL GCP_SIM_PUBLIC_URL \
  AZURE_CONSOLE_SIM_PUBLIC_URL AZURE_CLOUD_SIM_HOSTNAME CADDY_PORT \
  AZURE_FEDERATION_TENANT ADMIN_OIDC_CLIENT_SECRET AWS_OIDC_CLIENT_SECRET \
  GCP_OIDC_CLIENT_SECRET AZURE_OIDC_CLIENT_SECRET APPLICATION_RELEASE_REVISION \
  SHAUTH_BOOTSTRAP_ADMIN_EMAIL POSTGRES_PASSWORD; do
  [[ -n ${!name:-} ]] || {
    echo "error: $name must be set in deploy/.env" >&2
    exit 1
  }
done

# ---------------------------------------------------------------------------
# Step 1: compute SHAUTH_BOOTSTRAP_APPS_JSON and write deploy/.env.generated.
#
# Shauth reconciles this list at every startup (ReconcileBootstrapManagedApp
# + assertHydraClientReconciled upsert each entry by slug), so recomputing
# it and recreating the shauth container is safe to repeat any number of
# times — the registration is idempotent by construction, not by a
# check-then-create dance this script has to do itself. Backchannel logout
# URIs use the same public origin as everything else: Ory Hydra and every
# relying party share this compose network, and each service's compose
# network alias resolves identically for a container calling in and a
# browser calling in from the host (see deploy/.env.example), so there is no
# separate internal coordinate to keep in sync.
# ---------------------------------------------------------------------------
log "computing Shauth bootstrap-apps registration"
# Every URL, including backchannel_logout_uri, is the public https origin
# through caddy: Shauth itself rejects a non-HTTPS, non-loopback bootstrap
# app URL at startup (the same strict rule cmd/sockerless-admin/shauth.go and
# simulators/ui-auth/auth.go enforce on the relying-party side), so
# backchannel_logout_uri cannot be a plain-HTTP internal Docker service
# name. Ory Hydra therefore delivers backchannel logout through caddy too —
# see deploy/README.md "Security" for how the hydra service trusts caddy's
# local certificate authority to complete that TLS handshake.
bootstrap_apps=$(jq -cn \
  --arg admin_url "$ADMIN_PUBLIC_URL" --arg admin_secret "$ADMIN_OIDC_CLIENT_SECRET" \
  --arg aws_url "$AWS_SIM_PUBLIC_URL" --arg aws_secret "$AWS_OIDC_CLIENT_SECRET" \
  --arg gcp_url "$GCP_SIM_PUBLIC_URL" --arg gcp_secret "$GCP_OIDC_CLIENT_SECRET" \
  --arg azure_url "$AZURE_CONSOLE_SIM_PUBLIC_URL" --arg azure_secret "$AZURE_OIDC_CLIENT_SECRET" \
  --arg release "$APPLICATION_RELEASE_REVISION" '
  [
    {slug:"sockerless-admin",name:"Sockerless Admin",description:"Sockerless operator console",
     launch_url:($admin_url+"/ui/"),oidc_client_id:"sockerless-admin",oidc_client_secret:$admin_secret,
     redirect_uris:[$admin_url+"/auth/shauth/callback"],post_logout_redirect_uris:[$admin_url+"/auth/shauth/logout/complete"],
     frontchannel_logout_uri:($admin_url+"/auth/shauth/frontchannel-logout"),backchannel_logout_uri:($admin_url+"/auth/shauth/backchannel-logout"),
     health_url:($admin_url+"/healthz"),monitoring_url:($admin_url+"/ui/"),validation_url:($admin_url+"/auth/validation"),
     signed_out_url:($admin_url+"/auth/signed-out"),release_revision:$release},
    {slug:"sockerless-aws",name:"Sockerless AWS simulator",description:"Amazon Web Services simulator",
     launch_url:($aws_url+"/ui/"),oidc_client_id:"sockerless-aws",oidc_client_secret:$aws_secret,
     redirect_uris:[$aws_url+"/auth/oidc/callback"],post_logout_redirect_uris:[$aws_url+"/auth/shauth/logout/complete"],
     frontchannel_logout_uri:($aws_url+"/auth/oidc/frontchannel-logout"),backchannel_logout_uri:($aws_url+"/auth/oidc/backchannel-logout"),
     health_url:($aws_url+"/health"),monitoring_url:"",validation_url:($aws_url+"/auth/validation"),
     signed_out_url:($aws_url+"/auth/signed-out"),release_revision:$release},
    {slug:"sockerless-gcp",name:"Sockerless Google Cloud simulator",description:"Google Cloud simulator",
     launch_url:($gcp_url+"/ui/"),oidc_client_id:"sockerless-gcp",oidc_client_secret:$gcp_secret,
     redirect_uris:[$gcp_url+"/auth/oidc/callback"],post_logout_redirect_uris:[$gcp_url+"/auth/shauth/logout/complete"],
     frontchannel_logout_uri:($gcp_url+"/auth/oidc/frontchannel-logout"),backchannel_logout_uri:($gcp_url+"/auth/oidc/backchannel-logout"),
     health_url:($gcp_url+"/health"),monitoring_url:"",validation_url:($gcp_url+"/auth/validation"),
     signed_out_url:($gcp_url+"/auth/signed-out"),release_revision:$release},
    {slug:"sockerless-azure",name:"Sockerless Microsoft Azure simulator",description:"Microsoft Azure simulator (console)",
     launch_url:($azure_url+"/ui/"),oidc_client_id:"sockerless-azure",oidc_client_secret:$azure_secret,
     redirect_uris:[$azure_url+"/auth/oidc/callback"],post_logout_redirect_uris:[$azure_url+"/auth/shauth/logout/complete"],
     frontchannel_logout_uri:($azure_url+"/auth/oidc/frontchannel-logout"),backchannel_logout_uri:($azure_url+"/auth/oidc/backchannel-logout"),
     health_url:($azure_url+"/health"),monitoring_url:"",validation_url:($azure_url+"/auth/validation"),
     signed_out_url:($azure_url+"/auth/signed-out"),release_revision:$release}
  ]')

# Shauth and Ory Hydra each need a PostgreSQL DSN built from
# POSTGRES_PASSWORD. Deriving them here (instead of asking deploy/.env to
# repeat the password inside two more URLs) means the password has exactly
# one source of truth.
hydra_dsn="postgres://shauth:${POSTGRES_PASSWORD}@postgres:5432/hydra?sslmode=disable"
shauth_database_url="postgres://shauth:${POSTGRES_PASSWORD}@postgres:5432/shauth?sslmode=disable"

# Preserve any coordinate a previous run already wrote (the Azure clientId,
# below) while replacing the rest — .env.generated is fully regenerated
# from what this script knows plus what it discovers below. A placeholder
# stands in until Step 6 mints the real value: Compose interpolates every
# service's environment (including the "console"-profile-only
# simulator-azure-console) whenever *any* service is brought up, regardless
# of which services are actually requested or which profiles are active, so
# the base-profile boot in Step 2 would otherwise fail before it starts.
# simulator-azure-console itself is never created with the placeholder in
# place — it is not in Step 2's service list and its profile is not active
# until Step 7.
generated_azure_client_id="pending-provisioning"
if [[ -f .env.generated ]]; then
  existing=$(grep -m1 '^SOCKERLESS_CONSOLE_FEDERATION_CLIENT_ID=' .env.generated | cut -d= -f2- || true)
  [[ -n $existing ]] && generated_azure_client_id=$existing
fi
{
  printf 'HYDRA_DSN=%s\n' "$hydra_dsn"
  printf 'SHAUTH_DATABASE_URL=%s\n' "$shauth_database_url"
  printf 'SHAUTH_BOOTSTRAP_APPS_JSON=%s\n' "$bootstrap_apps"
  printf 'SOCKERLESS_CONSOLE_FEDERATION_CLIENT_ID=%s\n' "$generated_azure_client_id"
} >.env.generated

# ---------------------------------------------------------------------------
# Step 2: boot the base profile (everything except simulator-azure-console,
# which needs the clientId Step 5 mints) and wait for it to answer.
# ---------------------------------------------------------------------------
log "booting the base profile ($compose_file)"
compose up -d --wait --wait-timeout 300 postgres hydra-migrate hydra shauth-migrate shauth \
  sockerless-admin simulator-aws simulator-gcp simulator-azure-cloud caddy

# caddy issues every persistent origin's certificate from its own local
# certificate authority (deploy/Caddyfile's `tls internal`). Every curl call
# below that reaches a caddy-fronted https origin needs to trust it — export
# CURL_CA_BUNDLE once so every subsequent curl invocation in this script
# picks it up automatically, matching how a browser would need to trust the
# same authority once (see deploy/README.md "Security").
caddy_ca_file="$deploy_dir/.caddy-local-ca.crt"
log "extracting caddy's local certificate authority"
for _ in $(seq 1 60); do
  compose cp caddy:/data/caddy/pki/authorities/local/root.crt "$caddy_ca_file" 2>/dev/null && break
  sleep 1
done
[[ -s $caddy_ca_file ]] || {
  echo "error: caddy's local certificate authority never appeared at /data/caddy/pki/authorities/local/root.crt" >&2
  exit 1
}
export CURL_CA_BUNDLE="$caddy_ca_file"

wait_for_url() {
  local url=$1 name=$2 attempt=0
  while ((attempt < 180)); do
    if curl --fail --silent --output /dev/null "$url"; then
      return 0
    fi
    attempt=$((attempt + 1))
    sleep 1
  done
  echo "error: $name did not become ready at $url" >&2
  return 1
}
wait_for_url "$SHAUTH_PUBLIC_URL/healthz" Shauth
wait_for_url "$ADMIN_PUBLIC_URL/healthz" "Sockerless Admin"
wait_for_url "$AWS_SIM_PUBLIC_URL/health" "AWS simulator"
wait_for_url "$GCP_SIM_PUBLIC_URL/health" "Google Cloud simulator"
wait_for_url "https://$AZURE_CLOUD_SIM_HOSTNAME:$CADDY_PORT/health" "Microsoft Azure simulator (cloud)"

# ---------------------------------------------------------------------------
# Step 3: register `sockerless login` as a public Hydra client. It is
# deliberately not a Shauth managed app: managed apps are browser
# applications with health/validation/logout-bridge URLs a terminal cannot
# serve, so the CLI runs Shauth's explicit consent screen once instead of
# the managed-app auto-consent, over the RFC 8252 loopback redirect.
# ---------------------------------------------------------------------------
log "registering the sockerless CLI as a public Hydra client"
hydra_admin_url="http://127.0.0.1:${HYDRA_ADMIN_PORT:-4445}"
cli_client_status=$(curl --silent --output /dev/null --write-out '%{http_code}' "$hydra_admin_url/admin/clients/sockerless-cli")
if [[ $cli_client_status == 404 ]]; then
  curl --fail --silent --show-error --request POST --header 'Content-Type: application/json' \
    --data '{"client_id":"sockerless-cli","client_name":"Sockerless CLI","token_endpoint_auth_method":"none","grant_types":["authorization_code"],"response_types":["code"],"scope":"openid","redirect_uris":["http://127.0.0.1/callback"]}' \
    "$hydra_admin_url/admin/clients" >/dev/null
  log "  created sockerless-cli"
elif [[ $cli_client_status == 200 ]]; then
  log "  sockerless-cli already registered"
else
  echo "error: unexpected status $cli_client_status reading $hydra_admin_url/admin/clients/sockerless-cli" >&2
  exit 1
fi

# ---------------------------------------------------------------------------
# Step 4: AWS federation — an IAM OpenID Connect provider trusting Shauth,
# and the role(s) the console/CLI assume via AssumeRoleWithWebIdentity.
# The AWS simulator enforces SigV4 on its control plane exactly as real AWS
# does, so provisioning signs every call the way a real administrator client
# (aws CLI / SDK) would; this environment carries no aws CLI, so the
# signature is computed here from bash + openssl against the simulator's
# seeded bootstrap administrator credential (access-key/secret = test/test,
# region us-east-1) — the same well-known coordinate the SDK/CLI/Terraform
# test surfaces configure.
# ---------------------------------------------------------------------------
log "provisioning AWS federation"
aws_sim_host=${AWS_SIM_PUBLIC_URL#https://}
aws_admin_access_key="test"
aws_admin_secret_key="test"
aws_sigv4_region="us-east-1"

urlenc() { jq -rn --arg v "$1" '$v|@uri'; }
hex_sha256() { printf '%s' "$1" | openssl dgst -sha256 | sed -E 's/^.*=[[:space:]]*//'; }
hmac_hex() { printf '%s' "$2" | openssl dgst -sha256 -mac HMAC -macopt "hexkey:$1" | sed -E 's/^.*=[[:space:]]*//'; }

# aws_sigv4_post signs a form-encoded IAM control-plane request and posts it,
# tolerating AWS's own idempotency signal (EntityAlreadyExists) so this
# script can be re-run safely; any other non-2xx status is a real failure.
aws_sigv4_post() {
  local body=$1 host=$aws_sim_host service=iam
  local amzdate datestamp payload_hash signed_headers canonical_request scope string_to_sign
  amzdate=$(date -u +%Y%m%dT%H%M%SZ)
  datestamp=$(date -u +%Y%m%d)
  payload_hash=$(hex_sha256 "$body")
  signed_headers="content-type;host;x-amz-content-sha256;x-amz-date"
  canonical_request=$(printf 'POST\n/\n\ncontent-type:application/x-www-form-urlencoded\nhost:%s\nx-amz-content-sha256:%s\nx-amz-date:%s\n\n%s\n%s' \
    "$host" "$payload_hash" "$amzdate" "$signed_headers" "$payload_hash")
  scope="$datestamp/$aws_sigv4_region/$service/aws4_request"
  string_to_sign=$(printf 'AWS4-HMAC-SHA256\n%s\n%s\n%s' "$amzdate" "$scope" "$(hex_sha256 "$canonical_request")")
  local k_date k_region k_service k_signing signature
  k_date=$(printf '%s' "$datestamp" | openssl dgst -sha256 -hmac "AWS4$aws_admin_secret_key" | sed -E 's/^.*=[[:space:]]*//')
  k_region=$(hmac_hex "$k_date" "$aws_sigv4_region")
  k_service=$(hmac_hex "$k_region" "$service")
  k_signing=$(hmac_hex "$k_service" "aws4_request")
  signature=$(hmac_hex "$k_signing" "$string_to_sign")
  local response status
  response=$(curl --silent --show-error -X POST "https://$host/" \
    -H 'Content-Type: application/x-www-form-urlencoded' \
    -H "X-Amz-Date: $amzdate" \
    -H "X-Amz-Content-Sha256: $payload_hash" \
    -H "Authorization: AWS4-HMAC-SHA256 Credential=$aws_admin_access_key/$scope, SignedHeaders=$signed_headers, Signature=$signature" \
    --data "$body" --write-out $'\n%{http_code}')
  status=${response##*$'\n'}
  if [[ $status == 2* ]]; then
    return 0
  fi
  if [[ $status == 409 && $response == *EntityAlreadyExists* ]]; then
    return 0
  fi
  echo "error: AWS provisioning call failed (HTTP $status): $body" >&2
  echo "$response" >&2
  exit 1
}

# The trust policy is built with jq rather than hand-escaped inline JSON: a
# double-quoted string containing a backslash-escaped `[{...}]` array nested
# inside a $(...) command substitution trips a real bash 3.2 (macOS's
# default /bin/bash) command-substitution parsing bug that silently drops
# everything before the array — jq removes the escaping entirely, and there
# is exactly one federated-principal ARN to interpolate regardless of which
# role it is signed for.
web_identity_trust_policy=$(jq -cn --arg federated "arn:aws:iam::123456789012:oidc-provider/${SHAUTH_PUBLIC_URL#https://}" \
  '{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Federated":$federated},"Action":"sts:AssumeRoleWithWebIdentity"}]}')

aws_sigv4_post "Action=CreateOpenIDConnectProvider&Version=2010-05-08&Url=$(urlenc "$SHAUTH_PUBLIC_URL")&ClientIDList.member.1=sockerless-aws&ClientIDList.member.2=sockerless-cli&ThumbprintList.member.1=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
aws_sigv4_post "Action=CreateRole&Version=2010-05-08&RoleName=console-federation-role&AssumeRolePolicyDocument=$(urlenc "$web_identity_trust_policy")"
aws_sigv4_post "Action=CreateRole&Version=2010-05-08&RoleName=cli-federation-role&AssumeRolePolicyDocument=$(urlenc "$web_identity_trust_policy")"
aws_sigv4_post "Action=PutRolePolicy&Version=2010-05-08&RoleName=cli-federation-role&PolicyName=cli-access&PolicyDocument=$(urlenc '{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["ecs:*","lambda:*","ecr:*","s3:*","logs:*","sts:*"],"Resource":"*"}]}')"
# The console reads across services and administers IAM users/access keys
# from its credential-minting pages, so the role needs that access; without
# it the simulator's IAM enforcement denies the federated calls exactly as
# real AWS would deny an operator role never authorized for the IAM console.
aws_sigv4_post "Action=PutRolePolicy&Version=2010-05-08&RoleName=console-federation-role&PolicyName=console-access&PolicyDocument=$(urlenc '{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["ecs:*","lambda:*","ecr:*","s3:*","logs:*","iam:*","organizations:*","sts:*"],"Resource":"*"}]}')"

# ---------------------------------------------------------------------------
# Step 5: GCP federation — a workforce pool + OIDC provider trusting Shauth.
# The Google Cloud simulator enforces OAuth2 bearer authentication on its
# Identity and Access Management data plane exactly as real Google does, so
# provisioning first mints a bootstrap access token from the simulator's own
# token endpoint — the exempt minter a real client also uses to acquire a
# credential — then presents it as a bearer.
# ---------------------------------------------------------------------------
log "provisioning Google Cloud federation"
gcp_token=$(curl --silent --show-error --fail -X POST "$GCP_SIM_PUBLIC_URL/token" \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  --data-urlencode 'grant_type=client_credentials' \
  --data-urlencode 'scope=https://www.googleapis.com/auth/cloud-platform' | jq -r '.access_token')
[[ -n $gcp_token && $gcp_token != null ]] || {
  echo "error: Google Cloud simulator issued no bootstrap access token" >&2
  exit 1
}

gcp_post_idempotent() {
  local url=$1 data=$2 status
  status=$(curl --silent --show-error --output /dev/null --write-out '%{http_code}' -X POST "$url" \
    -H "Authorization: Bearer $gcp_token" -H 'Content-Type: application/json' -d "$data")
  if [[ $status == 2* || $status == 409 ]]; then
    return 0
  fi
  echo "error: GCP provisioning call to $url failed (HTTP $status)" >&2
  exit 1
}
gcp_post_idempotent "$GCP_SIM_PUBLIC_URL/v1/locations/global/workforcePools?workforcePoolId=sockerless-console" \
  '{"displayName":"Sockerless Console","parent":"organizations/sockerless"}'
gcp_post_idempotent "$GCP_SIM_PUBLIC_URL/v1/locations/global/workforcePools/sockerless-console/providers?workforcePoolProviderId=sso" \
  '{"displayName":"Shauth","oidc":{"issuerUri":"'"$SHAUTH_PUBLIC_URL"'","clientId":"sockerless-gcp"},"attributeMapping":{"google.subject":"assertion.sub"}}'
# `sockerless login` federates through its own provider in the same pool:
# the provider's clientId must equal the assertion audience, and the CLI's
# Shauth ID tokens carry aud=sockerless-cli.
gcp_post_idempotent "$GCP_SIM_PUBLIC_URL/v1/locations/global/workforcePools/sockerless-console/providers?workforcePoolProviderId=cli" \
  '{"displayName":"Shauth CLI","oidc":{"issuerUri":"'"$SHAUTH_PUBLIC_URL"'","clientId":"sockerless-cli"},"attributeMapping":{"google.subject":"assertion.sub"}}'

# ---------------------------------------------------------------------------
# Step 6: Azure federation — a resource group, a user-assigned managed
# identity (its clientId is the coordinate simulator-azure-console federates
# as), and a federated identity credential trusting the Shauth-signed-in
# operator. Microsoft Entra Workload Identity Federation pins exact
# subjects: real Azure requires one federated identity credential per
# trusted subject (up to 20 per identity), so this provisions the
# credential for the bootstrap administrator only — deploy/README.md
# "Security" documents provisioning an additional credential per operator.
# ---------------------------------------------------------------------------
log "provisioning Microsoft Azure federation"
azure_cloud_base="https://$AZURE_CLOUD_SIM_HOSTNAME:$CADDY_PORT"
azure_subscription=00000000-0000-0000-0000-000000000001
azure_rg=sockerless-console-rg
azure_identity=sockerless-console-identity

azure_arm_bearer=$(curl --silent --show-error --fail -X POST "$azure_cloud_base/$AZURE_FEDERATION_TENANT/oauth2/v2.0/token" \
  --data-urlencode grant_type=client_credentials \
  --data-urlencode client_id=test-client-id \
  --data-urlencode client_secret=test-client-secret \
  --data-urlencode 'scope=https://management.azure.com/.default' | jq -r '.access_token')
[[ -n $azure_arm_bearer && $azure_arm_bearer != null ]] || {
  echo "error: Microsoft Azure simulator issued no Azure Resource Manager token" >&2
  exit 1
}
azure_arm() {
  curl --silent --show-error --fail "$@" -H "Authorization: Bearer $azure_arm_bearer" -H 'Content-Type: application/json'
}
azure_arm -o /dev/null -X PUT \
  "$azure_cloud_base/subscriptions/$azure_subscription/resourcegroups/$azure_rg?api-version=2021-04-01" \
  -d '{"location":"eastus"}'
azure_client_id=$(azure_arm -X PUT \
  "$azure_cloud_base/subscriptions/$azure_subscription/resourceGroups/$azure_rg/providers/Microsoft.ManagedIdentity/userAssignedIdentities/$azure_identity?api-version=2023-01-31" \
  -d '{"location":"eastus"}' | jq -r '.properties.clientId')
[[ -n $azure_client_id && $azure_client_id != null ]] || {
  echo "error: the console's managed identity carried no clientId" >&2
  exit 1
}

shauth_admin_subject=$(compose exec -T postgres psql -U shauth -d shauth -Atc \
  "SELECT id FROM users WHERE email='${SHAUTH_BOOTSTRAP_ADMIN_EMAIL}'")
[[ -n $shauth_admin_subject ]] || {
  echo "error: bootstrap administrator $SHAUTH_BOOTSTRAP_ADMIN_EMAIL not found in the Shauth identity store" >&2
  exit 1
}
azure_arm -o /dev/null -X PUT \
  "$azure_cloud_base/subscriptions/$azure_subscription/resourceGroups/$azure_rg/providers/Microsoft.ManagedIdentity/userAssignedIdentities/$azure_identity/federatedIdentityCredentials/shauth-admin?api-version=2023-01-31" \
  -d "{\"properties\":{\"issuer\":\"$SHAUTH_PUBLIC_URL\",\"subject\":\"$shauth_admin_subject\",\"audiences\":[\"sockerless-azure\"]}}"

# ---------------------------------------------------------------------------
# Step 7: persist the minted clientId and boot simulator-azure-console.
# ---------------------------------------------------------------------------
log "writing deploy/.env.generated and starting the console profile"
{
  printf 'HYDRA_DSN=%s\n' "$hydra_dsn"
  printf 'SHAUTH_DATABASE_URL=%s\n' "$shauth_database_url"
  printf 'SHAUTH_BOOTSTRAP_APPS_JSON=%s\n' "$bootstrap_apps"
  printf 'SOCKERLESS_CONSOLE_FEDERATION_CLIENT_ID=%s\n' "$azure_client_id"
} >.env.generated

compose --profile console up -d --wait --wait-timeout 300 simulator-azure-console
wait_for_url "$AZURE_CONSOLE_SIM_PUBLIC_URL/health" "Microsoft Azure simulator (console)"

log "provisioning complete"
