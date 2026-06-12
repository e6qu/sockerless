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
#    registration token / labels / repo resolved from the invocation
#    event (per-invocation override) or the function env (dispatcher
#    spawn path).
# 4. After the runner exits (one job done), kills sockerless and
#    POSTs an empty response to the invocation.
# 5. Loop — Lambda may reuse this execution environment for the next
#    invocation; the runner's state is in /tmp + EFS so it's
#    appropriately isolated per invocation.

set -euo pipefail

RUNTIME_API="http://${AWS_LAMBDA_RUNTIME_API}/2018-06-01/runtime"

# --- idle gate (shared across runner images; edit all copies together) ---
# The dispatcher spawns one ephemeral runner per queued job, and GitHub
# may hand the job to a different runner (duplicate-spawn race / seen-set
# loss). Bound only the PRE-PICKUP window: if no job starts within
# RUNNER_IDLE_SECONDS (default 120) the runner exits cleanly; once a job
# is picked up (a Runner.Worker child appears) it runs unbounded by this
# gate, to the job's own timeout. A whole-process `timeout` would kill
# in-flight jobs; an absent gate leaves never-assigned runners waiting
# forever — on Lambda that burns the rest of the 15-min invocation.
job_started() {
  local d
  for d in /proc/[0-9]*; do
    if tr '\0' ' ' < "$d/cmdline" 2>/dev/null | grep -q 'Runner\.Worker'; then
      return 0
    fi
  done
  return 1
}

run_with_idle_gate() {
  local idle="${RUNNER_IDLE_SECONDS:-120}"
  local marker
  marker=$(mktemp)
  rm -f "$marker"
  "$@" &
  local run_pid=$!
  (
    deadline=$((SECONDS + idle))
    while [ "$SECONDS" -lt "$deadline" ]; do
      kill -0 "$run_pid" 2>/dev/null || exit 0
      job_started && exit 0
      sleep 2
    done
    job_started && exit 0
    echo "idle-gate: no job picked up within ${idle}s; stopping runner" >&2
    touch "$marker"
    kill "$run_pid" 2>/dev/null || true
  ) &
  local gate_pid=$!
  local rc=0
  wait "$run_pid" || rc=$?
  kill "$gate_pid" 2>/dev/null || true
  wait "$gate_pid" 2>/dev/null || true
  if [ -e "$marker" ]; then
    rm -f "$marker"
    return 0 # idle exit is the expected no-job outcome, not a failure
  fi
  return "$rc"
}
# --- end idle gate ---

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

  # Runner config, canonical names (the same RUNNER_REG_TOKEN /
  # RUNNER_REPO contract every runner image speaks). Two sources, in
  # precedence order:
  #   1. invocation-event fields (runner_reg_token / runner_repo /
  #      runner_name / runner_labels) — the per-invocation override
  #      the direct-invoke harness uses (the Lambda analog of ECS
  #      container overrides);
  #   2. function env — what a dispatcher spawning through
  #      sockerless-backend-lambda (`docker run -e RUNNER_…`) sets at
  #      CreateFunction.
  # Every field must resolve from one of the two; missing → error.
  local ev_reg_token ev_repo ev_name ev_labels
  ev_reg_token=$(jq -r '.runner_reg_token // empty' <<<"$event")
  ev_repo=$(jq -r '.runner_repo // empty' <<<"$event")
  ev_name=$(jq -r '.runner_name // empty' <<<"$event")
  ev_labels=$(jq -r '.runner_labels // empty' <<<"$event")
  RUNNER_REG_TOKEN="${ev_reg_token:-${RUNNER_REG_TOKEN:-}}"
  RUNNER_REPO="${ev_repo:-${RUNNER_REPO:-}}"
  RUNNER_NAME="${ev_name:-${RUNNER_NAME:-}}"
  RUNNER_LABELS="${ev_labels:-${RUNNER_LABELS:-}}"

  if [ -z "$RUNNER_REG_TOKEN" ] || [ -z "$RUNNER_REPO" ] || [ -z "$RUNNER_NAME" ] || [ -z "$RUNNER_LABELS" ]; then
    local err="missing runner config: need runner_reg_token/runner_repo/runner_name/runner_labels in the invocation event or RUNNER_REG_TOKEN/RUNNER_REPO/RUNNER_NAME/RUNNER_LABELS in the function env"
    curl -sS -X POST "${RUNTIME_API}/invocation/${request_id}/error" \
      -H 'Content-Type: application/json' \
      -d "{\"errorMessage\":\"${err}\",\"errorType\":\"BadEvent\"}"
    return 1
  fi
  export RUNNER_REG_TOKEN RUNNER_REPO RUNNER_NAME RUNNER_LABELS

  # Lambda's image filesystem is read-only except /tmp + EFS mount
  # (/mnt/runner-workspace). Stage the runner's working tree under
  # /tmp/runner-state (config.sh writes its registration files into
  # the working dir). _work and externals must live on EFS so the
  # sub-task Lambda — which mounts the same access point — can see
  # them; both go under EFS subpaths.
  mkdir -p /tmp/runner-state /mnt/runner-workspace/_work /mnt/runner-workspace/externals
  if [ ! -e /tmp/runner-state/run.sh ]; then
    # First invocation in this execution environment — copy the
    # actions/runner tree into /tmp (skipping _work and externals;
    # those go to EFS).
    echo "[bootstrap] staging runner working tree to /tmp/runner-state…"
    cp -a /opt/runner/. /tmp/runner-state/
  fi
  # Stage externals onto EFS once per filesystem. The sub-task Lambda
  # reads externals via its own FSC mount of the same AP, so it must
  # see the same files. Idempotent: skip when EFS already has the
  # node20 binary (sentinel for a populated externals tree).
  if [ ! -x /mnt/runner-workspace/externals/node20/bin/node ]; then
    echo "[bootstrap] staging externals to EFS…"
    cp -a /opt/runner/externals/. /mnt/runner-workspace/externals/
  fi
  # Symlink runner-side paths into the EFS subpaths.
  rm -rf /tmp/runner-state/_work /tmp/runner-state/externals
  ln -sfn /mnt/runner-workspace/_work /tmp/runner-state/_work
  ln -sfn /mnt/runner-workspace/externals /tmp/runner-state/externals

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
  # RUNNER_REG_TOKEN.
  rm -f .runner .credentials .credentials_rsaparams

  ./config.sh \
    --url "https://github.com/${RUNNER_REPO}" \
    --token "$RUNNER_REG_TOKEN" \
    --name "$RUNNER_NAME" \
    --labels "$RUNNER_LABELS" \
    --unattended --ephemeral --replace

  DOCKER_HOST=tcp://localhost:3375 run_with_idle_gate ./run.sh --once || true

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
