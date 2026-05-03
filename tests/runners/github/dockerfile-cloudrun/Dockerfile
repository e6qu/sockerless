# GitHub Actions runner image for sockerless cloudrun dispatch.
#
# Each `docker run` of this image (spawned by github-runner-dispatcher
# per queued workflow_job) brings up a long-lived runner container on
# Cloud Run that:
#  1. Starts `sockerless-backend-cloudrun` on localhost:3375 in the
#     background (DOCKER_HOST target for the GitHub runner's docker
#     executor).
#  2. Registers an ephemeral GitHub Actions runner with the configured
#     repo + label set via RUNNER_REG_TOKEN.
#  3. Polls GitHub for one job, runs it, exits.
#
# Per BUG-862 (backend ↔ host primitive must match), this image bakes
# `sockerless-backend-cloudrun` (NOT lambda or ecs); each step
# container the runner spawns lands as a Cloud Run Job via the in-image
# sockerless backend.

FROM ubuntu:22.04

ARG RUNNER_VERSION=2.334.0
ARG RUNNER_ARCH=x64
ARG DOCKER_VERSION=27.3.1
ARG DOCKER_ARCH=x86_64

# Runtime deps for the actions/runner + docker CLI.
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
      curl ca-certificates tar gzip jq sudo \
      iproute2 libicu70 git \
    && rm -rf /var/lib/apt/lists/*

# Static docker CLI — pure client. Talks to localhost:3375 (sockerless)
# inside the same container.
RUN curl -fsSL "https://download.docker.com/linux/static/stable/${DOCKER_ARCH}/docker-${DOCKER_VERSION}.tgz" \
    | tar xz --strip-components=1 -C /usr/local/bin docker/docker && \
    /usr/local/bin/docker --version

# Pre-built sockerless-backend-cloudrun (cross-compiled linux/amd64).
COPY sockerless-backend-cloudrun /usr/local/bin/sockerless-backend-cloudrun

# Phase 122g: cloudrun overlay-injects sockerless-cloudrun-bootstrap
# into every per-step Cloud Run Service image so `docker exec` can
# dispatch via Path B HTTP POST (specs/CLOUD_RESOURCE_MAPPING.md
# § Lesson 8). The bootstrap binary lives on the runner image's disk
# so the in-image cloudrun backend can stage it into Cloud Build
# contexts when building per-image overlays.
COPY sockerless-cloudrun-bootstrap /opt/sockerless/sockerless-cloudrun-bootstrap
RUN chmod +x /usr/local/bin/sockerless-backend-cloudrun \
             /opt/sockerless/sockerless-cloudrun-bootstrap

ENV SOCKERLESS_CLOUDRUN_BOOTSTRAP=/opt/sockerless/sockerless-cloudrun-bootstrap

# actions/runner extracted into /opt/runner.
RUN mkdir -p /opt/runner && \
    curl -fsSL \
      "https://github.com/actions/runner/releases/download/v${RUNNER_VERSION}/actions-runner-linux-${RUNNER_ARCH}-${RUNNER_VERSION}.tar.gz" \
      | tar xz -C /opt/runner && \
    /opt/runner/bin/installdependencies.sh

# Non-root runner user (actions/runner refuses to run as root).
RUN useradd -m -u 1001 runner && \
    chown -R runner:runner /opt/runner

COPY bootstrap.sh /usr/local/bin/bootstrap.sh
RUN chmod +x /usr/local/bin/bootstrap.sh

ENV DOCKER_HOST=tcp://localhost:3375
ENV RUNNER_LABELS=sockerless-cloudrun

ENTRYPOINT ["/usr/local/bin/bootstrap.sh"]
