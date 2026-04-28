#!/bin/bash
# Lambda Runtime API bootstrap that turns the `actions/runner` image
# into an invocable Lambda function. Each Lambda invocation:
#
# 1. Fetches the invocation event from
#    `${AWS_LAMBDA_RUNTIME_API}/2018-06-01/runtime/invocation/next`.
# 2. Starts `sockerless-backend-lambda` on `localhost:3375` in the
#    background (so the runner's docker calls dispatch each
#    `container:` sub-task as a fresh Lambda invocation — keeping
#    the workflow on Lambda primitives, per project rule "backend ↔
#    host primitive must match").
# 3. Configures + runs `actions/runner --ephemeral` with the
#    registration token / labels / repo URL pulled from the event
#    payload.
# 4. After the runner exits (one job done), kills sockerless and
#    POSTs an empty response to the invocation.
# 5. Loop — Lambda may reuse this execution environment for the next
#    invocation; the runner's state is in /tmp + EFS so it's
#    appropriately isolated per invocation.

set -euo pipefail

RUNTIME_API="http://${AWS_LAMBDA_RUNTIME_API}/2018-06-01/runtime"

handle_invocation() {
  local headers_file body_file request_id event
  headers_file=$(mktemp)
  body_file=$(mktemp)
  trap 'rm -f "$headers_file" "$body_file"' RETURN

  curl -sS -D "$headers_file" -o "$body_file" "${RUNTIME_API}/invocation/next"
  request_id=$(grep -i 'lambda-runtime-aws-request-id' "$headers_file" | awk '{print $2}' | tr -d '\r')

  if [ -z "$request_id" ]; then
    echo "FATAL: no Lambda-Runtime-Aws-Request-Id header in /next response" >&2
    return 1
  fi

  event=$(cat "$body_file")
  echo "[bootstrap] invocation request=${request_id}"

  # Pull runner config from the invocation event (the dispatcher
  # passes a JSON object with the registration token + labels +
  # repo URL).
  RUNNER_REPO_URL=$(jq -r '.runner_repo_url // empty' <<<"$event")
  RUNNER_TOKEN=$(jq -r '.runner_token // empty' <<<"$event")
  RUNNER_NAME=$(jq -r '.runner_name // empty' <<<"$event")
  RUNNER_LABELS=$(jq -r '.runner_labels // empty' <<<"$event")

  if [ -z "$RUNNER_REPO_URL" ] || [ -z "$RUNNER_TOKEN" ] || [ -z "$RUNNER_NAME" ] || [ -z "$RUNNER_LABELS" ]; then
    local err="missing required fields in invocation event (need runner_repo_url, runner_token, runner_name, runner_labels)"
    curl -sS -X POST "${RUNTIME_API}/invocation/${request_id}/error" \
      -H 'Content-Type: application/json' \
      -d "{\"errorMessage\":\"${err}\",\"errorType\":\"BadEvent\"}"
    return 1
  fi
  export RUNNER_REPO_URL RUNNER_TOKEN RUNNER_NAME RUNNER_LABELS

  # Lambda's image filesystem is read-only except /tmp + EFS mount
  # (/mnt/runner-workspace). Set up the runner's working tree under
  # /tmp/runner-state — it copies the actions/runner binary tree
  # there (config.sh writes its registration files into the working
  # dir) and symlinks _work / externals appropriately.
  mkdir -p /tmp/runner-state
  if [ ! -e /tmp/runner-state/run.sh ]; then
    # First invocation in this execution environment — populate
    # working tree from the image.
    echo "[bootstrap] staging runner working tree to /tmp/runner-state…"
    cp -a /opt/runner/. /tmp/runner-state/
  fi
  # Symlink _work → EFS-mounted workspace (shared with sub-tasks).
  rm -rf /tmp/runner-state/_work
  ln -sfn /mnt/runner-workspace /tmp/runner-state/_work
  # externals stays as the staged copy in /tmp/runner-state/externals
  # (already populated by the cp -a above; image's externals tree
  # comes along for the ride).

  # Start sockerless on localhost:3375. Reads its config from env
  # vars set by Terraform on the Lambda function.
  /usr/local/bin/sockerless-backend-lambda -addr :3375 -log-level debug 2>&1 \
    | sed -u 's/^/[sockerless] /' &
  local sockerless_pid=$!

  # Wait for sockerless ready.
  for _ in $(seq 1 60); do
    if curl -fsS http://localhost:3375/_ping > /dev/null 2>&1; then
      echo "[bootstrap] sockerless listening on :3375 (pid=${sockerless_pid})"
      break
    fi
    sleep 0.5
  done

  if ! curl -fsS http://localhost:3375/_ping > /dev/null 2>&1; then
    local err="sockerless-backend-lambda never became ready"
    curl -sS -X POST "${RUNTIME_API}/invocation/${request_id}/error" \
      -H 'Content-Type: application/json' \
      -d "{\"errorMessage\":\"${err}\",\"errorType\":\"BootstrapFailure\"}"
    kill "$sockerless_pid" 2>/dev/null || true
    return 1
  fi

  cd /tmp/runner-state

  # Lambda execution environments are reused across invocations.
  # The runner's config files (.runner, .credentials,
  # .credentials_rsaparams) persist in /tmp from a prior invocation
  # — but the registration on GitHub's side is auto-cleaned by the
  # ephemeral lifecycle. Remove the local state files so config.sh
  # creates a fresh registration matching the new RUNNER_NAME /
  # RUNNER_TOKEN.
  rm -f .runner .credentials .credentials_rsaparams

  ./config.sh \
    --url "$RUNNER_REPO_URL" \
    --token "$RUNNER_TOKEN" \
    --name "$RUNNER_NAME" \
    --labels "$RUNNER_LABELS" \
    --unattended --ephemeral --replace

  DOCKER_HOST=tcp://localhost:3375 ./run.sh || true

  # Stop sockerless.
  kill "$sockerless_pid" 2>/dev/null || true

  # Acknowledge the invocation. Empty body — the runner's job
  # output went to GitHub directly.
  curl -sS -X POST "${RUNTIME_API}/invocation/${request_id}/response" \
    -H 'Content-Type: application/json' \
    -d '{"status":"completed"}'
}

# Lambda may reuse the execution environment across invocations. Loop
# until the platform tears us down.
while true; do
  if ! handle_invocation; then
    # Initialization-error path is handled inside the function; loop
    # back to fetch the next invocation if any.
    sleep 1
  fi
done
