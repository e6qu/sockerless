#!/bin/sh
# gitlab-runner init script. Runs as a Cloud Run init container before
# the vanilla `gitlab/gitlab-runner` container starts. Idempotent —
# safe to re-run on every revision spin-up.
#
# Steps:
#   1. List + delete any pre-existing project-scoped runners whose
#      description matches our naming prefix (cleanup of prior revisions
#      so the GitLab project doesn't accumulate stale runners).
#   2. Create a fresh runner via POST /api/v4/user/runners with
#      runner_type=project_type. Capture the auth token from the response.
#   3. Render /shared/config.toml with the auth token + the cell-specific
#      tags + DOCKER_HOST=tcp://localhost:3375 (or 3376 — see SOCKERLESS_PORT).
#   4. Exit 0. The vanilla gitlab-runner container picks up
#      /shared/config.toml via its standard --config flag.
#
# Required env (set by the Cloud Run service spec):
#   GITLAB_TOKEN           — personal access token from Secret Manager (gitlab-pat)
#   GITLAB_URL             — gitlab base URL (default https://gitlab.com)
#   GITLAB_PROJECT_ID      — numeric project id (e.g. 81023556)
#   RUNNER_NAME_PREFIX     — used both to name THIS runner uniquely AND
#                            to identify stale runners to clean up
#   RUNNER_TAGS            — comma-separated runner tags (e.g. sockerless-cloudrun)
#   SOCKERLESS_PORT        — :3375 or :3376 (cloudrun vs gcf sidecar port)
#   SHARED_CONFIG_PATH     — defaults to /shared/config.toml
#
# Each call mints a fresh runner; old ones get cleaned up before the new
# one is created so the project's runner list stays bounded.

set -eu

: "${GITLAB_TOKEN:?missing GITLAB_TOKEN}"
: "${GITLAB_PROJECT_ID:?missing GITLAB_PROJECT_ID}"
: "${RUNNER_NAME_PREFIX:?missing RUNNER_NAME_PREFIX}"
: "${RUNNER_TAGS:?missing RUNNER_TAGS}"
: "${SOCKERLESS_PORT:?missing SOCKERLESS_PORT}"
GITLAB_URL="${GITLAB_URL:-https://gitlab.com}"
SHARED_CONFIG_PATH="${SHARED_CONFIG_PATH:-/shared/config.toml}"

API="${GITLAB_URL}/api/v4"
HDR="PRIVATE-TOKEN: ${GITLAB_TOKEN}"
RUNNER_NAME="${RUNNER_NAME_PREFIX}-$(date -u +%Y%m%dT%H%M%S)"

echo "[init] cleaning up stale (offline) project_type runners from project ${GITLAB_PROJECT_ID}"
# Delete every project_type runner whose status is offline. The 50-
# runner-per-project cap fills up quickly on iterative cell runs unless
# we aggressively reap. Online runners (ourselves from prior revision
# rollouts that are still mid-shutdown) are left alone — they go offline
# fast once the prior revision drains.
TMP_IDS=$(mktemp)
PAGE=1
while :; do
    RESP=$(curl -fsSL -H "${HDR}" "${API}/projects/${GITLAB_PROJECT_ID}/runners?per_page=100&page=${PAGE}&type=project_type&status=offline")
    BATCH=$(echo "${RESP}" | jq -r '.[].id')
    if [ -z "${BATCH}" ]; then
        break
    fi
    echo "${BATCH}" >> "${TMP_IDS}"
    PAGE=$((PAGE + 1))
    if [ ${PAGE} -gt 10 ]; then
        break
    fi
done
COUNT=$(wc -l < "${TMP_IDS}" | tr -d ' ')
echo "[init]   ${COUNT} stale runners to delete"
while IFS= read -r ID; do
    [ -z "${ID}" ] && continue
    STATUS=$(curl -fsS -o /dev/null -w "%{http_code}" -X DELETE -H "${HDR}" "${API}/runners/${ID}")
    if [ "${STATUS}" != "204" ]; then
        echo "[init]   (delete id=${ID} returned HTTP ${STATUS}; ignoring)"
    fi
done < "${TMP_IDS}"
rm -f "${TMP_IDS}"

echo "[init] registering fresh runner name=${RUNNER_NAME} tags=${RUNNER_TAGS}"
# Capture body + status code separately so a 4xx error surfaces the
# server's actual reason (400 with no body explanation is what cost an
# iteration before — the body is the diagnostic).
TMPRESP=$(mktemp)
HTTP_CODE=$(curl -sS -o "${TMPRESP}" -w "%{http_code}" -X POST -H "${HDR}" \
    --data-urlencode "runner_type=project_type" \
    --data-urlencode "project_id=${GITLAB_PROJECT_ID}" \
    --data-urlencode "description=${RUNNER_NAME}" \
    --data-urlencode "tag_list=${RUNNER_TAGS}" \
    "${API}/user/runners" || echo "curl-failed")
RUNNER_RESP=$(cat "${TMPRESP}")
rm -f "${TMPRESP}"
if [ "${HTTP_CODE}" != "201" ] && [ "${HTTP_CODE}" != "200" ]; then
    echo "[init] ERROR: gitlab POST /user/runners returned HTTP ${HTTP_CODE}; body: ${RUNNER_RESP}"
    exit 1
fi
TOKEN=$(echo "${RUNNER_RESP}" | jq -r '.token // empty')
if [ -z "${TOKEN}" ]; then
    echo "[init] ERROR: registration succeeded HTTP ${HTTP_CODE} but body has no .token: ${RUNNER_RESP}"
    exit 1
fi
echo "[init] runner registered (HTTP ${HTTP_CODE}), auth-token captured"

mkdir -p "$(dirname "${SHARED_CONFIG_PATH}")"
cat > "${SHARED_CONFIG_PATH}" <<EOF
concurrent = 1
check_interval = 0

[session_server]
  session_timeout = 1800

[[runners]]
  name = "${RUNNER_NAME}"
  url = "${GITLAB_URL}"
  token = "${TOKEN}"
  executor = "docker"
  [runners.docker]
    host = "tcp://localhost${SOCKERLESS_PORT}"
    image = "alpine:latest"
    pull_policy = "if-not-present"
    disable_cache = true
    wait_for_services_timeout = -1
  [runners.feature_flags]
    FF_NETWORK_PER_BUILD = true
EOF
echo "[init] wrote ${SHARED_CONFIG_PATH}"
echo "[init] init work complete; binding :8081 for Cloud Run startup probe + sleeping"

# Cloud Run requires a dependency container to be 'ready' (startup probe
# passing) — not 'exited'. Bind a tiny TCP listener on :8081 in the
# background so the spec's tcpSocket startup probe succeeds, then sleep
# forever. The runner container's container-dependencies annotation
# waits for this ready state before starting.
( while :; do nc -l -p 8081 >/dev/null 2>&1 || true; done ) &
wait
