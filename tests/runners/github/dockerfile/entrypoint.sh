#!/usr/bin/env bash
# Runner-container entrypoint. Reads its registration parameters from
# the env (passed by the test harness via `docker run -e ...`); the
# token is short-lived (~1h) and never persisted past container exit.
#
# The runner registers ephemeral so GitHub auto-deregisters it after
# one job. If the harness crashes mid-run, the runner side cleans
# itself up within 24h on GitHub's end.
set -euo pipefail

: "${RUNNER_REPO_URL:?RUNNER_REPO_URL not set}"
: "${RUNNER_TOKEN:?RUNNER_TOKEN not set}"
: "${RUNNER_NAME:?RUNNER_NAME not set}"
: "${RUNNER_LABELS:?RUNNER_LABELS not set}"

./config.sh \
  --url "$RUNNER_REPO_URL" \
  --token "$RUNNER_TOKEN" \
  --name "$RUNNER_NAME" \
  --labels "$RUNNER_LABELS" \
  --unattended --ephemeral --replace

exec ./run.sh
