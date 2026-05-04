#!/bin/bash
# bootstrap.sh — gitlab-runner-gcf entrypoint. Mirror of
# gitlab/dockerfile-cloudrun/bootstrap.sh; only the backend binary +
# port differ.
set -euo pipefail

# Auto-discover sockerless config from GCP instance metadata.
META=http://metadata.google.internal/computeMetadata/v1
HDR='Metadata-Flavor: Google'
export SOCKERLESS_GCF_PROJECT=$(curl -sf -H "$HDR" $META/project/project-id)
export SOCKERLESS_GCF_REGION=$(curl -sf -H "$HDR" $META/instance/region | awk -F/ '{print $NF}')
export SOCKERLESS_GCP_BUILD_BUCKET="${SOCKERLESS_GCF_PROJECT}-build"
export SOCKERLESS_GCP_SHARED_VOLUMES="runner-workspace=/tmp/runner-work=${SOCKERLESS_GCF_PROJECT}-runner-workspace,runner-externals=/opt/runner/externals=${SOCKERLESS_GCF_PROJECT}-runner-workspace"
echo "bootstrap: project=$SOCKERLESS_GCF_PROJECT region=$SOCKERLESS_GCF_REGION"

nohup /usr/local/bin/sockerless-backend-gcf -addr :3376 -log-level debug \
    > >(tee /tmp/sockerless-backend.log >&2) 2>&1 &

deadline=$((SECONDS + 30))
until curl -sfo /dev/null http://localhost:3376/_ping; do
    if [ $SECONDS -ge $deadline ]; then
        echo "bootstrap: sockerless-backend-gcf did not become ready in 30s"
        cat /tmp/sockerless-backend.log >&2 || true
        exit 1
    fi
    sleep 1
done
echo "bootstrap: sockerless-backend-gcf ready"

# BUG-913: gitlab-runner needs --working-directory to exist; create it.
mkdir -p /tmp/runner-work

if [ -n "${PORT:-}" ]; then
    nohup socat "TCP-LISTEN:${PORT},reuseaddr,fork" "TCP:127.0.0.1:3376" \
        >/tmp/socat.log 2>&1 &
    echo "bootstrap: socat \$PORT=${PORT} → sockerless-backend-gcf:3376"
fi

gitlab-runner register \
    --non-interactive \
    --url "${GITLAB_URL:-https://gitlab.com}" \
    --token "${GITLAB_RUNNER_TOKEN}" \
    --name "${GITLAB_RUNNER_NAME:-$(hostname)}" \
    --executor docker \
    --docker-image alpine:latest \
    --docker-host "tcp://localhost:3376" \
    --docker-pull-policy if-not-present

# BUG-915: post-edit to ensure disable_cache=true.
sed -i 's/disable_cache = false/disable_cache = true/' /etc/gitlab-runner/config.toml

# BUG-918 wedge: pin helper_image to tag form (avoids sha256: digest
# refs that sockerless's image-resolve mangles).
sed -i '/\[runners.docker\]/a\
    helper_image = "registry.gitlab.com/gitlab-org/gitlab-runner/gitlab-runner-helper:x86_64-v17.5.0"' \
    /etc/gitlab-runner/config.toml

# BUG-925: skip the wait-for-services healthcheck container (postgres
# sidecar will be deployed in the same Cloud Run Function as the BUILD
# container, so the redundant TCP probe wedge isn't needed).
sed -i '/\[runners.docker\]/a\
    wait_for_services_timeout = -1' \
    /etc/gitlab-runner/config.toml

# BUG-925: enable FF_NETWORK_PER_BUILD so gitlab-runner uses standard
# Docker user-defined networks (verified in v17.5
# executors/docker/services.go::createServices). The gcf backend's
# network-pod auto-detector requires this signal.
cat >> /etc/gitlab-runner/config.toml <<'EOF'

  [runners.feature_flags]
    FF_NETWORK_PER_BUILD = true
EOF

# /cache stays — sockerless gcf backend mounts via Cloud Run Service
# Volume{Gcs{Bucket}} (gcf is backed by Cloud Run Service per
# CLOUD_RESOURCE_MAPPING.md). No wedge.

cat /etc/gitlab-runner/config.toml

exec gitlab-runner run --config /etc/gitlab-runner/config.toml --working-directory /tmp/runner-work
