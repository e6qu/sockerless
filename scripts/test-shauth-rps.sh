#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
shauth_root=${SHAUTH_SOURCE_DIR:?SHAUTH_SOURCE_DIR must point to a Shauth checkout}
shauth_expected_commit=${SHAUTH_EXPECTED_COMMIT:?SHAUTH_EXPECTED_COMMIT must identify the exact clean Shauth revision}

for command in bun curl docker git jq make node openssl; do
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
    {slug:"sockerless-admin",name:"Sockerless Admin",description:"Sockerless operator console",launch_url:"http://localhost:29090/ui/",oidc_client_id:"sockerless-admin",oidc_client_secret:$admin,redirect_uris:["http://localhost:29090/auth/shauth/callback"],post_logout_redirect_uris:["http://localhost:29090/auth/signed-out"],frontchannel_logout_uri:"http://localhost:29090/auth/shauth/frontchannel-logout",backchannel_logout_uri:"http://localhost:29090/auth/shauth/backchannel-logout",health_url:"http://localhost:29090/healthz",monitoring_url:"http://localhost:29090/ui/",validation_url:"http://localhost:29090/auth/validation",signed_out_url:"http://localhost:29090/auth/signed-out",release_revision:$release},
    {slug:"sockerless-aws",name:"Sockerless AWS simulator",description:"Amazon Web Services simulator",launch_url:"http://localhost:29310/ui/",oidc_client_id:"sockerless-aws",oidc_client_secret:$aws,redirect_uris:["http://localhost:29310/auth/oidc/callback"],post_logout_redirect_uris:["http://localhost:29310/auth/signed-out"],frontchannel_logout_uri:"http://localhost:29310/auth/oidc/frontchannel-logout",backchannel_logout_uri:"http://localhost:29310/auth/oidc/backchannel-logout",health_url:"http://localhost:29310/health",monitoring_url:"",validation_url:"http://localhost:29310/auth/validation",signed_out_url:"http://localhost:29310/auth/signed-out",release_revision:$release},
    {slug:"sockerless-gcp",name:"Sockerless Google Cloud simulator",description:"Google Cloud simulator",launch_url:"http://localhost:29320/ui/",oidc_client_id:"sockerless-gcp",oidc_client_secret:$gcp,redirect_uris:["http://localhost:29320/auth/oidc/callback"],post_logout_redirect_uris:["http://localhost:29320/auth/signed-out"],frontchannel_logout_uri:"http://localhost:29320/auth/oidc/frontchannel-logout",backchannel_logout_uri:"http://localhost:29320/auth/oidc/backchannel-logout",health_url:"http://localhost:29320/health",monitoring_url:"",validation_url:"http://localhost:29320/auth/validation",signed_out_url:"http://localhost:29320/auth/signed-out",release_revision:$release},
    {slug:"sockerless-azure",name:"Sockerless Microsoft Azure simulator",description:"Microsoft Azure simulator",launch_url:"http://localhost:29330/ui/",oidc_client_id:"sockerless-azure",oidc_client_secret:$azure,redirect_uris:["http://localhost:29330/auth/oidc/callback"],post_logout_redirect_uris:["http://localhost:29330/auth/signed-out"],frontchannel_logout_uri:"http://localhost:29330/auth/oidc/frontchannel-logout",backchannel_logout_uri:"http://localhost:29330/auth/oidc/backchannel-logout",health_url:"http://localhost:29330/health",monitoring_url:"",validation_url:"http://localhost:29330/auth/validation",signed_out_url:"http://localhost:29330/auth/signed-out",release_revision:$release}
  ]')

compose() {
  SHAUTH_VALIDATOR_TOKEN="$validator_token" SHAUTH_VALIDATION_STATUS_TOKEN="$validation_status_token" \
    docker compose --project-name "$compose_project" --project-directory "$shauth_root" \
    -f "$shauth_root/compose.yaml" "$@"
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
compose up --build --detach

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

(cd "$repo_root/ui" && bun install --frozen-lockfile)
for package in admin simulator-aws simulator-gcp simulator-azure; do
  (cd "$repo_root/ui/packages/$package" && bun run build)
done
make -C "$repo_root/cmd/sockerless-admin" build
make -C "$repo_root/simulators/aws" build
make -C "$repo_root/simulators/gcp" build
make -C "$repo_root/simulators/azure" build

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
  APPLICATION_RELEASE_REVISION="$6" \
    "$binary" >"$log_file" 2>&1 &
  pids+=("$!")
}

start_simulator "$repo_root/simulators/aws/simulator-aws" 29310 sockerless-aws "$aws_client_secret" "$work_dir/aws.log" "$source_revision"
start_simulator "$repo_root/simulators/gcp/simulator-gcp" 29320 sockerless-gcp "$gcp_client_secret" "$work_dir/gcp.log" "$source_revision"
start_simulator "$repo_root/simulators/azure/simulator-azure" 29330 sockerless-azure "$azure_client_secret" "$work_dir/azure.log" "$source_revision"

wait_for_url http://localhost:29090/healthz "Sockerless Admin"
wait_for_url http://localhost:29310/health "AWS simulator"
wait_for_url http://localhost:29320/health "Google Cloud simulator"
wait_for_url http://localhost:29330/health "Microsoft Azure simulator"

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
(cd "$shauth_root" && go build -o "$work_dir/shauth-validator" ./cmd/shauth-validator)
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
