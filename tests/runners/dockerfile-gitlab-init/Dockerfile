# gitlab-runner registration init container. Runs ONCE per Cloud Run
# revision spin-up before the vanilla `gitlab/gitlab-runner` container
# starts (Cloud Run dependsOn ordering). Cleans stale runners + mints
# a fresh one + writes config.toml to a shared volume the runner reads.
#
# Smallest viable base for `curl + jq + sh`. Alpine is plenty.
#
# Build + push:
#   podman build --platform=linux/amd64 \
#     -t us-central1-docker.pkg.dev/sockerless-live-46x3zg4imo/sockerless-live/gitlab-runner-init:latest \
#     -f Dockerfile .
#   podman push us-central1-docker.pkg.dev/sockerless-live-46x3zg4imo/sockerless-live/gitlab-runner-init:latest

FROM alpine:3.20

RUN apk add --no-cache curl jq

COPY register.sh /usr/local/bin/register.sh
RUN chmod +x /usr/local/bin/register.sh

ENTRYPOINT ["/usr/local/bin/register.sh"]
