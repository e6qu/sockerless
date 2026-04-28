#!/bin/bash
# Lambda Runtime API bootstrap that turns the `actions/runner` image
# into an invocable Lambda function. Each Lambda invocation:
#
# 1. Fetches the invocation event from
#    `${AWS_LAMBDA_RUNTIME_API}/2018-06-01/runtime/invocation/next`.
# 2. Starts `sockerless-backend-ecs` on `localhost:3375` in the
#    background (so the runner's docker calls resolve to ECS RunTask
#    sub-task spawns).
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

  # Lambda mounts EFS under /mnt/ only — symlink the runner's
  # workspace to the actual mount. Runner thinks
  # /home/runner/_work exists at the canonical path; reads/writes
  # go to /mnt/runner-workspace which is EFS.
  mkdir -p /home/runner
  if [ ! -L /home/runner/_work ]; then
    rm -rf /home/runner/_work
    ln -sfn /mnt/runner-workspace /home/runner/_work
  fi

  # Externals: Lambda's single file_system_config rule means we can
  # only mount one access point. Externals lives in the read-only
  # image layer at /opt/runner/externals; the runner reads from
  # /home/runner/externals → /opt/runner/externals via symlink.
  mkdir -p /home/runner
  if [ ! -L /home/runner/externals ] && [ ! -d /home/runner/externals ]; then
    ln -sfn /opt/runner/externals /home/runner/externals
  fi

  # Start sockerless on localhost:3375. Reads its config from env
  # vars set by Terraform on the Lambda function.
  /usr/local/bin/sockerless-backend-ecs -addr :3375 -log-level debug 2>&1 \
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
    local err="sockerless-backend-ecs never became ready"
    curl -sS -X POST "${RUNTIME_API}/invocation/${request_id}/error" \
      -H 'Content-Type: application/json' \
      -d "{\"errorMessage\":\"${err}\",\"errorType\":\"BootstrapFailure\"}"
    kill "$sockerless_pid" 2>/dev/null || true
    return 1
  fi

  # The runner config files are written to /opt/runner by config.sh.
  # /opt is read-only on Lambda — we need a writable dir. Symlink the
  # runner state into /tmp and run from there.
  if [ ! -L /home/runner/work-area ]; then
    rm -rf /tmp/runner-state
    mkdir -p /tmp/runner-state
    cp -a /opt/runner/. /tmp/runner-state/
    ln -sfn /tmp/runner-state /home/runner/work-area
  fi
  cd /home/runner/work-area

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
