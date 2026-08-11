#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
shauth_source_dir=${SHAUTH_SOURCE_DIR:?SHAUTH_SOURCE_DIR must point to a Shauth checkout}
shauth_root=$(cd "$shauth_source_dir" && pwd -P)
shauth_expected_commit=0fda680cba964e5768ed75a9c3e5b7230c418ca6

for command in bun curl docker git go jq make node openssl; do
  command -v "$command" >/dev/null 2>&1 || {
    echo "$command is required" >&2
    exit 1
  }
done
[[ -f "$shauth_root/compose.yaml" ]] || {
  echo "SHAUTH_SOURCE_DIR does not contain compose.yaml" >&2
  exit 1
}
shauth_actual_commit=$(git -C "$shauth_root" rev-parse HEAD)
if [[ ! $shauth_expected_commit =~ ^[0-9a-f]{40}$ || $shauth_actual_commit != "$shauth_expected_commit" ]]; then
  echo "Shauth checkout is not the expected immutable revision: expected ${shauth_expected_commit}, found ${shauth_actual_commit}" >&2
  exit 1
fi
if [[ -n $(git -C "$shauth_root" status --porcelain --untracked-files=no) ]]; then
  echo "Shauth checkout contains tracked changes; validation requires the exact clean revision" >&2
  exit 1
fi

work_dir=$(mktemp -d)
postgres_password=$(openssl rand -hex 32)
hydra_secret=$(openssl rand -base64 48 | tr -d '\n')
admin_password=$(openssl rand -base64 48 | tr -d '\n')
developer_password=$(openssl rand -base64 48 | tr -d '\n')
validator_token=$(openssl rand -base64 48 | tr -d '\n')
validation_status_token=$(openssl rand -base64 48 | tr -d '\n')
admin_client_secret=$(openssl rand -hex 32)
aws_client_secret=$(openssl rand -hex 32)
gcp_client_secret=$(openssl rand -hex 32)
azure_client_secret=$(openssl rand -hex 32)
session_secret=$(openssl rand -hex 32)
source_revision=$(git -C "$repo_root" rev-parse HEAD)
compose_project=sockerless-shauth-rps
pids=()

bootstrap_apps=$(jq -cn \
  --arg admin "$admin_client_secret" \
  --arg aws "$aws_client_secret" \
  --arg gcp "$gcp_client_secret" \
  --arg azure "$azure_client_secret" \
  --arg release "$source_revision" '
  [
    {slug:"sockerless-admin",name:"Sockerless Admin",description:"Sockerless operator console",launch_url:"http://localhost:29090/ui/",oidc_client_id:"sockerless-admin",oidc_client_secret:$admin,redirect_uris:["http://localhost:29090/auth/shauth/callback"],post_logout_redirect_uris:["http://localhost:29090/auth/shauth/logout/complete"],frontchannel_logout_uri:"http://localhost:29090/auth/shauth/frontchannel-logout",backchannel_logout_uri:"http://localhost:29090/auth/shauth/backchannel-logout",health_url:"http://localhost:29090/healthz",monitoring_url:"http://localhost:29090/ui/",validation_url:"http://localhost:29090/auth/validation",signed_out_url:"http://localhost:29090/auth/signed-out",release_revision:$release},
    {slug:"sockerless-aws",name:"Sockerless AWS simulator",description:"Amazon Web Services simulator",launch_url:"http://localhost:29310/ui/",oidc_client_id:"sockerless-aws",oidc_client_secret:$aws,redirect_uris:["http://localhost:29310/auth/oidc/callback"],post_logout_redirect_uris:["http://localhost:29310/auth/shauth/logout/complete"],frontchannel_logout_uri:"http://localhost:29310/auth/oidc/frontchannel-logout",backchannel_logout_uri:"http://localhost:29310/auth/oidc/backchannel-logout",health_url:"http://localhost:29310/health",monitoring_url:"",validation_url:"http://localhost:29310/auth/validation",signed_out_url:"http://localhost:29310/auth/signed-out",release_revision:$release},
    {slug:"sockerless-gcp",name:"Sockerless Google Cloud simulator",description:"Google Cloud simulator",launch_url:"http://localhost:29320/ui/",oidc_client_id:"sockerless-gcp",oidc_client_secret:$gcp,redirect_uris:["http://localhost:29320/auth/oidc/callback"],post_logout_redirect_uris:["http://localhost:29320/auth/shauth/logout/complete"],frontchannel_logout_uri:"http://localhost:29320/auth/oidc/frontchannel-logout",backchannel_logout_uri:"http://localhost:29320/auth/oidc/backchannel-logout",health_url:"http://localhost:29320/health",monitoring_url:"",validation_url:"http://localhost:29320/auth/validation",signed_out_url:"http://localhost:29320/auth/signed-out",release_revision:$release},
    {slug:"sockerless-azure",name:"Sockerless Microsoft Azure simulator",description:"Microsoft Azure simulator",launch_url:"http://localhost:29330/ui/",oidc_client_id:"sockerless-azure",oidc_client_secret:$azure,redirect_uris:["http://localhost:29330/auth/oidc/callback"],post_logout_redirect_uris:["http://localhost:29330/auth/shauth/logout/complete"],frontchannel_logout_uri:"http://localhost:29330/auth/oidc/frontchannel-logout",backchannel_logout_uri:"http://localhost:29330/auth/oidc/backchannel-logout",health_url:"http://localhost:29330/health",monitoring_url:"",validation_url:"http://localhost:29330/auth/validation",signed_out_url:"http://localhost:29330/auth/signed-out",release_revision:$release}
  ]')

compose() {
  SHAUTH_VALIDATOR_TOKEN="$validator_token" SHAUTH_VALIDATION_STATUS_TOKEN="$validation_status_token" \
    docker compose --project-name "$compose_project" --project-directory "$shauth_root" \
    -f "$shauth_root/compose.yaml" "$@"
}

compose_up_with_retry() {
  local attempt=1
  local max_attempts=4
  local retry_delay
  while ! compose up --build --detach; do
    if ((attempt >= max_attempts)); then
      echo "docker compose up failed after ${max_attempts} attempts" >&2
      return 1
    fi
    retry_delay=$((attempt * 5))
    echo "::warning::docker compose up attempt ${attempt} failed; retrying the same real stack in ${retry_delay}s" >&2
    sleep "$retry_delay"
    attempt=$((attempt + 1))
  done
}

cleanup() {
  status=$?
  trap - EXIT INT TERM
  if ((${#pids[@]} > 0)); then
    for pid in "${pids[@]}"; do
      kill "$pid" 2>/dev/null || true
    done
    for pid in "${pids[@]}"; do
      wait "$pid" 2>/dev/null || true
    done
  fi
  if [[ $status -ne 0 ]]; then
    for log_file in "$work_dir"/*.log; do
      [[ -f "$log_file" ]] && { echo "== $log_file ==" >&2; tail -n 30 "$log_file" >&2; }
    done
    compose logs --no-color --tail=40 shauth hydra >&2 || true
  fi
  compose down --volumes --remove-orphans >/dev/null 2>&1 || true
  rm -rf "$work_dir"
  exit "$status"
}
trap cleanup EXIT INT TERM

export POSTGRES_PASSWORD="$postgres_password"
export HYDRA_SYSTEM_SECRET="$hydra_secret"
export HYDRA_DSN="postgres://shauth:${postgres_password}@postgres:5432/hydra?sslmode=disable"
export HYDRA_PUBLIC_URL=http://localhost:8080
export SHAUTH_PUBLIC_URL=http://localhost:8080
export SHAUTH_DATABASE_URL="postgres://shauth:${postgres_password}@postgres:5432/shauth?sslmode=disable"
export GITHUB_CLIENT_ID=sockerless-integration
export GITHUB_CLIENT_SECRET=sockerless-integration-secret
export SHAUTH_BOOTSTRAP_ADMIN_PASSWORD="$admin_password"
export SHAUTH_BOOTSTRAP_APPS_JSON="$bootstrap_apps"

compose down --volumes --remove-orphans >/dev/null 2>&1 || true
compose_up_with_retry

wait_for_url() {
  local url=$1
  local name=$2
  local attempt=0
  while ((attempt < 180)); do
    if curl --fail --silent "$url" >/dev/null 2>&1; then
      return 0
    fi
    attempt=$((attempt + 1))
    sleep 1
  done
  echo "$name did not become ready at $url" >&2
  return 1
}

wait_for_url http://localhost:8080/healthz Shauth
wait_for_url http://localhost:4444/health/ready "Ory Hydra"

# The browser and every relying party use their public loopback coordinates.
# Ory Hydra runs in Docker, so only its server-to-server delivery coordinate is
# rewritten to Docker's host gateway after Shauth has reconciled each client.
for client_coordinate in \
  sockerless-admin:29090:/auth/shauth/backchannel-logout \
  sockerless-aws:29310:/auth/oidc/backchannel-logout \
  sockerless-gcp:29320:/auth/oidc/backchannel-logout \
  sockerless-azure:29330:/auth/oidc/backchannel-logout; do
  IFS=: read -r client_id port path <<<"$client_coordinate"
  registration=$(curl --fail --silent --show-error "http://localhost:4445/admin/clients/$client_id")
  registration=$(jq --arg uri "http://host.docker.internal:${port}${path}" '.backchannel_logout_uri = $uri' <<<"$registration")
  curl --fail --silent --show-error --request PUT --header 'Content-Type: application/json' \
    --data "$registration" "http://localhost:4445/admin/clients/$client_id" >/dev/null
done

# `sockerless login` is an RFC 8252 native app: the administrator registers it
# directly with Ory Hydra as a PUBLIC client (token_endpoint_auth_method
# "none", PKCE only) whose registered loopback redirect
# http://127.0.0.1/callback matches the CLI's ephemeral loopback port — the
# same admin surface the back-channel rewrite above uses. It is deliberately
# not a Shauth managed app: managed apps are browser applications with health,
# validation, and logout-bridge URLs a terminal cannot serve, so the CLI's
# consent runs through Shauth's explicit consent screen instead of the
# managed-app auto-consent.
curl --fail --silent --show-error --request POST --header 'Content-Type: application/json' \
  --data '{"client_id":"sockerless-cli","client_name":"Sockerless CLI","token_endpoint_auth_method":"none","grant_types":["authorization_code"],"response_types":["code"],"scope":"openid","redirect_uris":["http://127.0.0.1/callback"]}' \
  http://localhost:4445/admin/clients >/dev/null

(cd "$repo_root/ui" && bun install --frozen-lockfile)
(cd "$repo_root/ui/packages/admin" && bun run build)
make -C "$repo_root/cmd/sockerless-admin" build
# The simulators come from the sockerless-cloud repository at the version
# pinned in tests/go.mod; their console SPAs ship inside the module (committed
# dist/), so a plain module build embeds the consoles without a bun step.
mkdir -p "$repo_root/tests/.build"
for cloud in aws gcp azure; do
  (cd "$repo_root/tests" && go build -o ".build/simulator-$cloud" "github.com/e6qu/sockerless-cloud/simulator-$cloud")
done

mkdir -p "$work_dir/admin-home"
(
  cd "$work_dir/admin-home"
  exec env \
    SOCKERLESS_HOME="$work_dir/admin-home" \
    SOCKERLESS_ADMIN_SHAUTH_ISSUER=http://localhost:8080 \
    SOCKERLESS_ADMIN_SHAUTH_CLIENT_ID=sockerless-admin \
    SOCKERLESS_ADMIN_SHAUTH_CLIENT_SECRET="$admin_client_secret" \
    SOCKERLESS_ADMIN_SESSION_SECRET="$session_secret" \
    SOCKERLESS_ADMIN_PUBLIC_URL=http://localhost:29090 \
    SOCKERLESS_ADMIN_INSECURE_COOKIES=true \
    APPLICATION_RELEASE_REVISION="$source_revision" \
    "$repo_root/cmd/sockerless-admin/sockerless-admin" -addr :29090
) >"$work_dir/admin.log" 2>&1 &
pids+=("$!")

start_simulator() {
  local binary=$1
  local port=$2
  local client_id=$3
  local client_secret=$4
  local log_file=$5
  SIM_LISTEN_ADDR=":$port" \
  SIM_UI_OIDC_ISSUER=http://localhost:8080 \
  SIM_UI_OIDC_CLIENT_ID="$client_id" \
  SIM_UI_OIDC_CLIENT_SECRET="$client_secret" \
  SIM_UI_PUBLIC_URL="http://localhost:$port" \
  SIM_UI_SESSION_SECRET="$session_secret" \
  SIM_UI_INSECURE_COOKIES=true \
  SIM_DNS_PORT=0 \
  APPLICATION_RELEASE_REVISION="$6" \
  SOCKERLESS_CONSOLE_FEDERATION_AUDIENCE="${7:-}" \
    "$binary" >"$log_file" 2>&1 &
  pids+=("$!")
}

# The console federates the signed-in operator through a workforce pool provider
# that trusts Shauth. An administrator provisions that provider through the real
# Identity and Access Management API; the harness stands in for the
# administrator, and the provider's resource name is the coordinate the console
# federates against.
gcp_workforce_provider="//iam.googleapis.com/locations/global/workforcePools/sockerless-console/providers/sso"
provision_gcp_workforce_provider() {
  local base=http://localhost:29320
  # The Google Cloud simulator enforces OAuth2 bearer authentication on its
  # Identity and Access Management data plane exactly as real Google does: a
  # request without a valid access token is rejected with 401 UNAUTHENTICATED.
  # The administrator provisioning below therefore obtains an access token from
  # the simulator's token endpoint — the exempt minter a real client also uses
  # to acquire a credential — and presents it as a bearer, differing from real
  # Google only in the endpoint coordinate.
  local token
  token=$(curl --silent --show-error --fail -X POST "$base/token" \
    -H 'Content-Type: application/x-www-form-urlencoded' \
    --data-urlencode 'grant_type=client_credentials' \
    --data-urlencode 'scope=https://www.googleapis.com/auth/cloud-platform' | jq -r '.access_token')
  [[ -n $token && $token != null ]] || {
    echo "Google Cloud simulator issued no access token for provisioning" >&2
    return 1
  }
  curl --silent --show-error --fail -o /dev/null -X POST \
    "$base/v1/locations/global/workforcePools?workforcePoolId=sockerless-console" \
    -H "Authorization: Bearer $token" \
    -H 'Content-Type: application/json' \
    -d '{"displayName":"Sockerless Console","parent":"organizations/sockerless"}'
  curl --silent --show-error --fail -o /dev/null -X POST \
    "$base/v1/locations/global/workforcePools/sockerless-console/providers?workforcePoolProviderId=sso" \
    -H "Authorization: Bearer $token" \
    -H 'Content-Type: application/json' \
    -d '{"displayName":"Shauth","oidc":{"issuerUri":"http://localhost:8080","clientId":"sockerless-gcp"},"attributeMapping":{"google.subject":"assertion.sub"}}'
  # `sockerless login` federates through its own provider in the same pool:
  # the workforce provider's clientId must equal the assertion audience, and
  # the CLI's Shauth ID tokens carry aud=sockerless-cli.
  curl --silent --show-error --fail -o /dev/null -X POST \
    "$base/v1/locations/global/workforcePools/sockerless-console/providers?workforcePoolProviderId=cli" \
    -H "Authorization: Bearer $token" \
    -H 'Content-Type: application/json' \
    -d '{"displayName":"Shauth CLI","oidc":{"issuerUri":"http://localhost:8080","clientId":"sockerless-cli"},"attributeMapping":{"google.subject":"assertion.sub"}}'
}

# The AWS console federates through an IAM OpenID Connect provider that trusts
# Shauth and a role that trusts the provider — the federation an administrator
# provisions. The role ARN is the coordinate the console assumes.
aws_federation_role="arn:aws:iam::123456789012:role/console-federation-role"

# The AWS simulator enforces SigV4 on its control plane exactly as real AWS
# does: a request that does not carry a valid AWS4-HMAC-SHA256 signature over
# the request is rejected before any identity is trusted. The administrator
# provisioning below therefore signs every call the way a real admin client
# (aws CLI / SDK) does. The RPS CI job carries no aws CLI, so the signature is
# computed here from bash + openssl against the simulator's seeded bootstrap
# administrator credential (access-key/secret = test/test, region us-east-1),
# the same well-known coordinate the SDK/CLI/Terraform test surfaces configure.
aws_admin_access_key="test"
aws_admin_secret_key="test"
aws_sigv4_region="us-east-1"
aws_sigv4_service=iam

# urlenc percent-encodes a form value per RFC 3986 so the exact body bytes are
# both what curl sends and what the signature is computed over.
urlenc() { jq -rn --arg v "$1" '$v|@uri'; }

# hex_sha256 and hmac_hex are the two openssl primitives SigV4 needs. hmac_hex
# takes a hex-encoded key so the HMAC-SHA256 derivation chain can thread each
# intermediate key to the next without shell-hostile binary in a variable.
hex_sha256() { printf '%s' "$1" | openssl dgst -sha256 | sed -E 's/^.*=[[:space:]]*//'; }
hmac_hex() { printf '%s' "$2" | openssl dgst -sha256 -mac HMAC -macopt "hexkey:$1" | sed -E 's/^.*=[[:space:]]*//'; }

# aws_sigv4_post signs an already-form-encoded body with SigV4 and POSTs it to
# the AWS simulator's control-plane endpoint, matching the canonical request the
# simulator (and real AWS) reconstruct: method POST, canonical URI "/", empty
# canonical query, the four signed headers below, and the SHA-256 of the body as
# the declared payload hash. The signing key is derived from the bootstrap
# admin's secret through the AWS4 → date → region → iam → aws4_request HMAC chain.
aws_sigv4_post() {
  local body=$1
  local host=localhost:29310
  local amzdate datestamp payload_hash signed_headers canonical_request scope string_to_sign
  amzdate=$(date -u +%Y%m%dT%H%M%SZ)
  datestamp=$(date -u +%Y%m%d)
  payload_hash=$(hex_sha256 "$body")
  signed_headers="content-type;host;x-amz-content-sha256;x-amz-date"
  canonical_request=$(printf 'POST\n/\n\ncontent-type:application/x-www-form-urlencoded\nhost:%s\nx-amz-content-sha256:%s\nx-amz-date:%s\n\n%s\n%s' \
    "$host" "$payload_hash" "$amzdate" "$signed_headers" "$payload_hash")
  scope="$datestamp/$aws_sigv4_region/$aws_sigv4_service/aws4_request"
  string_to_sign=$(printf 'AWS4-HMAC-SHA256\n%s\n%s\n%s' "$amzdate" "$scope" "$(hex_sha256 "$canonical_request")")
  local k_date k_region k_service k_signing signature
  k_date=$(printf '%s' "$datestamp" | openssl dgst -sha256 -hmac "AWS4$aws_admin_secret_key" | sed -E 's/^.*=[[:space:]]*//')
  k_region=$(hmac_hex "$k_date" "$aws_sigv4_region")
  k_service=$(hmac_hex "$k_region" "$aws_sigv4_service")
  k_signing=$(hmac_hex "$k_service" "aws4_request")
  signature=$(hmac_hex "$k_signing" "$string_to_sign")
  curl --silent --show-error --fail -o /dev/null -X POST "http://$host/" \
    -H 'Content-Type: application/x-www-form-urlencoded' \
    -H "X-Amz-Date: $amzdate" \
    -H "X-Amz-Content-Sha256: $payload_hash" \
    -H "Authorization: AWS4-HMAC-SHA256 Credential=$aws_admin_access_key/$scope, SignedHeaders=$signed_headers, Signature=$signature" \
    --data "$body"
}

provision_aws_federation() {
  # One OpenID Connect provider per issuer; its client ID list carries every
  # audience Shauth issues tokens for — the console's and the CLI's.
  aws_sigv4_post "Action=CreateOpenIDConnectProvider&Version=2010-05-08&Url=$(urlenc http://localhost:8080)&ClientIDList.member.1=sockerless-aws&ClientIDList.member.2=sockerless-cli&ThumbprintList.member.1=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
  aws_sigv4_post "Action=CreateRole&Version=2010-05-08&RoleName=console-federation-role&AssumeRolePolicyDocument=$(urlenc '{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Federated":"arn:aws:iam::123456789012:oidc-provider/localhost:8080"},"Action":"sts:AssumeRoleWithWebIdentity"}]}')"
  # The role `sockerless login` writes into the aws CLI profile: the aws CLI
  # itself runs AssumeRoleWithWebIdentity with the Shauth ID token on demand.
  aws_sigv4_post "Action=CreateRole&Version=2010-05-08&RoleName=cli-federation-role&AssumeRolePolicyDocument=$(urlenc '{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Federated":"arn:aws:iam::123456789012:oidc-provider/localhost:8080"},"Action":"sts:AssumeRoleWithWebIdentity"}]}')"
  aws_sigv4_post "Action=PutRolePolicy&Version=2010-05-08&RoleName=cli-federation-role&PolicyName=cli-access&PolicyDocument=$(urlenc '{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["ecs:*","lambda:*","ecr:*","s3:*","logs:*","sts:*"],"Resource":"*"}]}')"
  # Amazon Data Firehose uses its own service role to reach destination
  # resources. The authenticated browser flow selects this real IAM role; the
  # simulator does not manufacture a role or bypass the service trust.
  aws_sigv4_post "Action=CreateRole&Version=2010-05-08&RoleName=console-firehose-role&AssumeRolePolicyDocument=$(urlenc '{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"firehose.amazonaws.com"},"Action":"sts:AssumeRole"}]}')"
  aws_sigv4_post "Action=PutRolePolicy&Version=2010-05-08&RoleName=console-firehose-role&PolicyName=firehose-destination-access&PolicyDocument=$(urlenc '{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:GetBucketLocation","s3:ListBucket","s3:PutObject"],"Resource":"*"}]}')"
  # The console reads across services and administers IAM users and access
  # keys from its credential-minting pages, so the administrator grants the
  # role that access; without it, the simulator's IAM enforcement denies the
  # federated calls, exactly as real AWS would deny an operator role that was
  # never authorized for the IAM console.
  # The console surfaces every AWS service the simulator implements, so the
  # operator's federated role carries each of those service prefixes. The list
  # is explicit rather than a wildcard: it documents exactly what the console
  # needs, and a service the console starts calling without being added here
  # fails loudly with the real AccessDenied instead of silently widening.
  aws_sigv4_post "Action=PutRolePolicy&Version=2010-05-08&RoleName=console-federation-role&PolicyName=console-access&PolicyDocument=$(urlenc '{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["acm:*","acm-pca:*","amplify:*","apigateway:*","autoscaling:*","batch:*","budgets:*","cloudfront:*","cloudtrail:*","cloudwatch:*","codebuild:*","dynamodb:*","ec2:*","ecr:*","ecs:*","elasticache:*","elasticfilesystem:*","elasticloadbalancing:*","events:*","firehose:*","glue:*","iam:*","kinesis:*","kms:*","lambda:*","logs:*","organizations:*","rds:*","route53:*","s3:*","scheduler:*","secretsmanager:*","servicediscovery:*","sns:*","sqs:*","ssm:*","states:*","sts:*","wafv2:*"],"Resource":"*"}]}')"
}

start_simulator "$repo_root/tests/.build/simulator-aws" 29310 sockerless-aws "$aws_client_secret" "$work_dir/aws.log" "$source_revision" "$aws_federation_role"
start_simulator "$repo_root/tests/.build/simulator-gcp" 29320 sockerless-gcp "$gcp_client_secret" "$work_dir/gcp.log" "$source_revision" "$gcp_workforce_provider"

wait_for_url http://localhost:29090/healthz "Sockerless Admin"
wait_for_url http://localhost:29310/health "AWS simulator"
provision_aws_federation
wait_for_url http://localhost:29320/health "Google Cloud simulator"
provision_gcp_workforce_provider

# ---- Microsoft Azure console: cloud and console as separate processes --------
# Real Microsoft Entra serves no cross-origin response for the client_credentials
# federation grant, so the console federates the operator server-side (the
# /auth/federation/token broker) rather than in the browser. The broker exchanges
# the operator's assertion at the cloud's Microsoft Entra endpoint, and the
# browser then reads Azure Resource Manager and Microsoft Graph directly — so the
# console must point at a *separate* cloud process, the way a real deployment
# does: a token the cloud mints is verified by that same cloud (each simulator
# instance signs with its own key), and the browser reaching the cloud
# cross-origin needs the cloud's real CORS. The cloud instance is plain HTTP so
# the console's server-side broker can reach it on macOS too (Go ignores
# SSL_CERT_FILE there); az's separate TLS listener below is unrelated.
azure_tenant=11111111-1111-1111-1111-111111111111
azure_subscription=00000000-0000-0000-0000-000000000001
shauth_admin_subject=$(compose exec -T postgres psql -U shauth -d shauth -Atc \
  "SELECT id FROM users WHERE email='admin@localhost.test'")
[[ -n $shauth_admin_subject ]] || {
  echo "bootstrap administrator not found in the Shauth identity store" >&2
  exit 1
}

azure_console_cloud_port=29332
azure_console_cloud=http://localhost:$azure_console_cloud_port
SIM_LISTEN_ADDR=":$azure_console_cloud_port" \
  "$repo_root/tests/.build/simulator-azure" >"$work_dir/azure-cloud.log" 2>&1 &
pids+=("$!")
wait_for_url "$azure_console_cloud/health" "Microsoft Azure console cloud"

# The administrator provisions the console's federated identity through the real
# Azure Resource Manager API, authenticating with the simulator's seeded
# bootstrap service principal — the same well-known coordinate the CLI test
# surfaces configure — then registers a federated identity credential for the
# operator's Shauth subject. The console presents the operator's assertion —
# issued to the console's own OpenID Connect client — so the credential's
# audience is that client id.
azure_console_arm_bearer=$(curl --silent --show-error --fail \
  -X POST "$azure_console_cloud/$azure_tenant/oauth2/v2.0/token" \
  --data-urlencode 'grant_type=client_credentials' \
  --data-urlencode 'client_id=test-client-id' \
  --data-urlencode 'client_secret=test-client-secret' \
  --data-urlencode 'scope=https://management.azure.com/.default' | jq -r '.access_token')
[[ -n $azure_console_arm_bearer && $azure_console_arm_bearer != null ]] || {
  echo "Microsoft Azure console cloud issued no Azure Resource Manager token" >&2
  exit 1
}
azure_console_arm() {
  curl --silent --show-error --fail -H "Authorization: Bearer $azure_console_arm_bearer" \
    -H 'Content-Type: application/json' "$@"
}
azure_console_arm -o /dev/null -X PUT \
  "$azure_console_cloud/subscriptions/$azure_subscription/resourcegroups/console-federation-rg?api-version=2021-04-01" \
  -d '{"location":"eastus"}'
azure_console_client_id=$(azure_console_arm -X PUT \
  "$azure_console_cloud/subscriptions/$azure_subscription/resourceGroups/console-federation-rg/providers/Microsoft.ManagedIdentity/userAssignedIdentities/console-identity?api-version=2023-01-31" \
  -d '{"location":"eastus"}' | jq -r '.properties.clientId')
[[ -n $azure_console_client_id && $azure_console_client_id != null ]] || {
  echo "console user-assigned identity carried no clientId" >&2
  exit 1
}
azure_console_arm -o /dev/null -X PUT \
  "$azure_console_cloud/subscriptions/$azure_subscription/resourceGroups/console-federation-rg/providers/Microsoft.ManagedIdentity/userAssignedIdentities/console-identity/federatedIdentityCredentials/shauth-operator?api-version=2023-01-31" \
  -d "{\"properties\":{\"issuer\":\"http://localhost:8080\",\"subject\":\"$shauth_admin_subject\",\"audiences\":[\"sockerless-azure\"]}}"

# Seed a Container Apps managed environment and job so the operator has a real
# resource to open in the portal — the browser flow proves the resource detail
# blade renders live cloud data end to end (federated ARM read), not just its
# shell. Provisioned as an administrator through the real Azure Resource Manager
# API on the same cloud process the console reads.
azure_console_env_id="/subscriptions/$azure_subscription/resourceGroups/console-federation-rg/providers/Microsoft.App/managedEnvironments/console-demo-env"
azure_console_arm -o /dev/null -X PUT \
  "$azure_console_cloud$azure_console_env_id?api-version=2024-03-01" \
  -d '{"location":"eastus","properties":{}}'
azure_console_job=console-demo-job
azure_console_arm -o /dev/null -X PUT \
  "$azure_console_cloud/subscriptions/$azure_subscription/resourceGroups/console-federation-rg/providers/Microsoft.App/jobs/$azure_console_job?api-version=2024-03-01" \
  -d "{\"location\":\"eastus\",\"properties\":{\"environmentId\":\"$azure_console_env_id\",\"configuration\":{\"triggerType\":\"Manual\",\"replicaTimeout\":60},\"template\":{\"containers\":[{\"name\":\"worker\",\"image\":\"alpine:latest\",\"command\":[\"sh\",\"-c\"],\"args\":[\"sleep 30\"]}]}}}"

# The console (UI + auth) starts only now that its cloud identity exists: it
# reads the generated client id at startup and points every cloud coordinate at
# the separate cloud process.
SIM_LISTEN_ADDR=":29330" \
SIM_UI_OIDC_ISSUER=http://localhost:8080 \
SIM_UI_OIDC_CLIENT_ID=sockerless-azure \
SIM_UI_OIDC_CLIENT_SECRET="$azure_client_secret" \
SIM_UI_PUBLIC_URL="http://localhost:29330" \
SIM_UI_SESSION_SECRET="$session_secret" \
SIM_UI_INSECURE_COOKIES=true \
APPLICATION_RELEASE_REVISION="$source_revision" \
SOCKERLESS_CONSOLE_CLOUD_API_ENDPOINT="$azure_console_cloud" \
SOCKERLESS_CONSOLE_GRAPH_API_ENDPOINT="$azure_console_cloud" \
SOCKERLESS_CONSOLE_LOGS_API_ENDPOINT="$azure_console_cloud" \
SOCKERLESS_CONSOLE_FEDERATION_ENDPOINT="$azure_console_cloud" \
SOCKERLESS_CONSOLE_FEDERATION_TENANT="$azure_tenant" \
SOCKERLESS_CONSOLE_FEDERATION_CLIENT_ID="$azure_console_client_id" \
  "$repo_root/tests/.build/simulator-azure" >"$work_dir/azure.log" 2>&1 &
pids+=("$!")
wait_for_url http://localhost:29330/health "Microsoft Azure simulator"

# ---- `sockerless login` coordinates -----------------------------------------
# The az CLI refuses an http authority: MSAL validates Microsoft Entra
# coordinates as HTTPS. The CLI login therefore targets a TLS listener of the
# same Microsoft Azure simulator binary at its own https coordinate, trusted
# through a harness-generated certificate the az CLI reads via
# REQUESTS_CA_BUNDLE — the trust bundle is a coordinate like the endpoints.
azure_cli_port=29331
azure_cli_base="https://127.0.0.1:$azure_cli_port"
# azure_tenant / azure_subscription / shauth_admin_subject are defined once with
# the Azure console two-process setup above and reused here for the CLI login.
openssl req -x509 -newkey rsa:2048 -keyout "$work_dir/azure-cli-tls.key" \
  -out "$work_dir/azure-cli-tls.crt" -days 7 -nodes -subj "/CN=localhost" \
  -addext "subjectAltName=DNS:localhost,IP:127.0.0.1" >/dev/null 2>&1
SIM_LISTEN_ADDR=":$azure_cli_port" \
SIM_TLS_CERT="$work_dir/azure-cli-tls.crt" \
SIM_TLS_KEY="$work_dir/azure-cli-tls.key" \
  "$repo_root/tests/.build/simulator-azure" >"$work_dir/azure-cli.log" 2>&1 &
pids+=("$!")

azure_curl() { curl --silent --show-error --fail --cacert "$work_dir/azure-cli-tls.crt" "$@"; }
for attempt in $(seq 1 60); do
  azure_curl -o /dev/null "$azure_cli_base/health" 2>/dev/null && break
  [[ $attempt == 60 ]] && { echo "Microsoft Azure simulator (TLS) did not become ready at $azure_cli_base" >&2; exit 1; }
  sleep 1
done

# The administrator provisions the CLI's federation resources through the real
# Azure Resource Manager API, authenticating with the simulator's seeded
# bootstrap service principal — the same well-known coordinate the CLI test
# surfaces configure.
azure_arm_bearer=$(azure_curl -X POST "$azure_cli_base/$azure_tenant/oauth2/v2.0/token" \
  --data-urlencode grant_type=client_credentials \
  --data-urlencode client_id=test-client-id \
  --data-urlencode client_secret=test-client-secret \
  --data-urlencode 'scope=https://management.azure.com/.default' | jq -r '.access_token')
[[ -n $azure_arm_bearer && $azure_arm_bearer != null ]] || {
  echo "Microsoft Azure simulator issued no Azure Resource Manager token for provisioning" >&2
  exit 1
}
azure_curl -o /dev/null -X PUT \
  "$azure_cli_base/subscriptions/$azure_subscription/resourcegroups/cli-login-rg?api-version=2021-04-01" \
  -H "Authorization: Bearer $azure_arm_bearer" -H 'Content-Type: application/json' -d '{"location":"eastus"}'
azure_cli_client_id=$(azure_curl -X PUT \
  "$azure_cli_base/subscriptions/$azure_subscription/resourceGroups/cli-login-rg/providers/Microsoft.ManagedIdentity/userAssignedIdentities/cli-login-identity?api-version=2023-01-31" \
  -H "Authorization: Bearer $azure_arm_bearer" -H 'Content-Type: application/json' -d '{"location":"eastus"}' |
  jq -r '.properties.clientId')
[[ -n $azure_cli_client_id && $azure_cli_client_id != null ]] || {
  echo "user-assigned identity for the CLI login carried no clientId" >&2
  exit 1
}

# Microsoft Entra Workload Identity Federation pins exact subjects. The
# relying-party matrix signs the CLI in as the bootstrap administrator, whose
# subject ($shauth_admin_subject, resolved with the console setup above) the
# federated identity credential trusts.
azure_curl -o /dev/null -X PUT \
  "$azure_cli_base/subscriptions/$azure_subscription/resourceGroups/cli-login-rg/providers/Microsoft.ManagedIdentity/userAssignedIdentities/cli-login-identity/federatedIdentityCredentials/shauth-admin?api-version=2023-01-31" \
  -H "Authorization: Bearer $azure_arm_bearer" -H 'Content-Type: application/json' \
  -d "{\"properties\":{\"issuer\":\"http://localhost:8080\",\"subject\":\"$shauth_admin_subject\",\"audiences\":[\"sockerless-cli\"]}}"

(cd "$repo_root/cmd/sockerless" && GOWORK=off go build -o "$work_dir/sockerless" .)
mkdir -p "$work_dir/cli-home"
cat >"$work_dir/cli-home/config.yaml" <<EOF
environments:
  rps:
    backend: docker
    login:
      issuer: http://localhost:8080
      client_id: sockerless-cli
    aws:
      region: us-east-1
      login:
        role_arn: arn:aws:iam::123456789012:role/cli-federation-role
        endpoint_url: http://localhost:29310
    gcp:
      project: sockerless
      login:
        workforce_audience: //iam.googleapis.com/locations/global/workforcePools/sockerless-console/providers/cli
        sts_endpoint: http://localhost:29320
        api_endpoint: http://localhost:29320
    azure:
      subscription_id: $azure_subscription
      login:
        tenant: $azure_tenant
        client_id: $azure_cli_client_id
        authority_endpoint: $azure_cli_base
        resource_manager_endpoint: $azure_cli_base
        ca_bundle: $work_dir/azure-cli-tls.crt
EOF
printf 'rps\n' >"$work_dir/cli-home/active"

# Coordinates the relying-party matrix uses to run and verify `sockerless
# login`: the built CLI, its isolated home, and isolated vendor-tool state so
# the flows never touch the invoking user's real ~/.aws, gcloud, or az config.
export SOCKERLESS_CLI_BIN="$work_dir/sockerless"
export SOCKERLESS_CLI_HOME="$work_dir/cli-home"
export SOCKERLESS_CLI_CONTEXT=rps
export SOCKERLESS_CLI_AWS_CONFIG_FILE="$work_dir/cli-home/aws-config"
export SOCKERLESS_CLI_CLOUDSDK_CONFIG="$work_dir/cli-home/gcloud"
export SOCKERLESS_CLI_AZURE_CONFIG_DIR="$work_dir/cli-home/azure"
export SOCKERLESS_CLI_AZURE_CA_BUNDLE="$work_dir/azure-cli-tls.crt"
export SOCKERLESS_CLI_AZURE_ARM_ENDPOINT="$azure_cli_base"
export SOCKERLESS_CLI_AZURE_FEDERATION_CLIENT_ID="$azure_cli_client_id"
export SOCKERLESS_RPS_AZURE_JOB="$azure_console_job"
# ---- end `sockerless login` coordinates -------------------------------------

assert_anonymous_validation() {
  local origin=$1
  local headers
  headers=$(curl --silent --show-error --dump-header - --output /dev/null --max-redirs 0 \
    "$origin/auth/validation")
  printf '%s' "$headers" | grep -Eq '^HTTP/[0-9.]+ 303'
  printf '%s' "$headers" | grep -Eqi '^location: /auth/signed-out'
}
assert_anonymous_validation http://localhost:29090
assert_anonymous_validation http://localhost:29310
assert_anonymous_validation http://localhost:29320
assert_anonymous_validation http://localhost:29330

for pid in "${pids[@]}"; do
  if ps eww -p "$pid" -o command= | grep -Eq 'SHAUTH_VALIDATOR_TOKEN=|SHAUTH_VALIDATION_STATUS_TOKEN=|SHAUTH_VALIDATION_USERNAME=|SHAUTH_VALIDATION_EMAIL=|SHAUTH_VALIDATION_PASSWORD='; then
    echo "relying-party process ${pid} inherited Shauth validation credentials" >&2
    exit 1
  fi
done

SHAUTH_BOOTSTRAP_ADMIN_PASSWORD="$admin_password" \
SHAUTH_DEVELOPER_PASSWORD="$developer_password" \
  node "$repo_root/ui/e2e/shauth-rps.mjs"

(cd "$shauth_root" && npm ci)
(cd "$shauth_root" && GOWORK=off go build -o "$work_dir/shauth-validator" ./cmd/shauth-validator)
SHAUTH_URL=http://localhost:8080 \
SHAUTH_VALIDATOR_TOKEN="$validator_token" \
SHAUTH_VALIDATION_USERNAME=shauth-validator \
SHAUTH_VALIDATION_EMAIL=shauth-validator@localhost.test \
SHAUTH_VALIDATOR_SCRIPT="$shauth_root/validator/validate.mjs" \
  "$work_dir/shauth-validator" >"$work_dir/validator.log" 2>&1 &
validator_pid=$!
pids+=("$validator_pid")

passed_count=0
failed_count=0
for ((attempt = 0; attempt < 720; attempt += 1)); do
  passed_count=$(compose exec -T postgres psql -U shauth -d shauth -Atc "SELECT count(*) FROM app_validation_runs WHERE status='passed'")
  failed_count=$(compose exec -T postgres psql -U shauth -d shauth -Atc "SELECT count(*) FROM app_validation_runs WHERE status='failed'")
  if [[ $failed_count != 0 || $passed_count == 8 ]]; then
    break
  fi
  if ! kill -0 "$validator_pid" 2>/dev/null; then
    wait "$validator_pid" 2>/dev/null || true
    echo "Shauth validator exited before completing the eight-flow matrix" >&2
    exit 1
  fi
  sleep 1
done
if [[ $failed_count != 0 || $passed_count != 8 ]]; then
  compose exec -T postgres psql -U shauth -d shauth \
    -c "SELECT app_slug,direction,status,failure FROM app_validation_runs ORDER BY requested_at,id" >&2
  exit 1
fi
validation_matrix=$(compose exec -T postgres psql -U shauth -d shauth -Atc \
  "SELECT string_agg(app_slug||':'||direction||':'||release_revision,',' ORDER BY app_slug,direction) FROM app_validation_runs WHERE status='passed'")
expected_validation_matrix="sockerless-admin:from_app:${source_revision},sockerless-admin:from_shauth:${source_revision},sockerless-aws:from_app:${source_revision},sockerless-aws:from_shauth:${source_revision},sockerless-azure:from_app:${source_revision},sockerless-azure:from_shauth:${source_revision},sockerless-gcp:from_app:${source_revision},sockerless-gcp:from_shauth:${source_revision}"
if [[ $validation_matrix != "$expected_validation_matrix" ]]; then
  echo "unexpected Shauth validation matrix: $validation_matrix" >&2
  exit 1
fi
kill "$validator_pid"
wait "$validator_pid" 2>/dev/null || true

grep -q 'accepted Shauth back-channel logout' "$work_dir/admin.log"
grep -q 'accepted Shauth back-channel logout' "$work_dir/aws.log"
grep -q 'accepted Shauth back-channel logout' "$work_dir/gcp.log"
grep -q 'accepted Shauth back-channel logout' "$work_dir/azure.log"
