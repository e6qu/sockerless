# Vanilla GitHub Actions runner image. NO sockerless code; binary is
# the upstream actions/runner tarball, COPY'd in unmodified. Image
# itself is published to AR by us (sockerless owns the publish, the
# content is unmodified upstream — this is just a CDN mirror so we
# don't pull from github.com on every spawn).
#
# Base: mcr.microsoft.com/dotnet/runtime-deps:8.0 — the Microsoft
# image that ships exactly the .NET runtime deps actions/runner needs
# (per their installdependencies.sh script which apt-installs
# dotnet-runtime-deps anyway). Using their image avoids the apt step
# entirely.
#
# DOCKER_HOST is set to tcp://localhost:3375 — the sockerless-backend
# sidecar in the same multi-container Cloud Run revision listens there.
# (gcf cell uses 3376 — pass --build-arg SOCKERLESS_PORT=3376.)
#
# Per-execution env override (set by the dispatcher when it RunJob's
# this image): RUNNER_REG_TOKEN, RUNNER_NAME, RUNNER_LABELS, RUNNER_REPO.
# That's the dispatcher's ONLY interface to the runner.
#
# Build + push (one-time, version-pinned):
#   podman build --platform=linux/amd64 \
#     --build-arg RUNNER_VERSION=2.334.0 \
#     -t us-central1-docker.pkg.dev/sockerless-live-46x3zg4imo/sockerless-live/github-runner-vanilla:2.334.0 \
#     -f Dockerfile .
#   podman push us-central1-docker.pkg.dev/sockerless-live-46x3zg4imo/sockerless-live/github-runner-vanilla:2.334.0

ARG RUNNER_VERSION=2.334.0
ARG RUNNER_ARCH=x64

# Stage 1: download + extract the actions/runner tarball.
FROM alpine:3.20 AS fetch
ARG RUNNER_VERSION
ARG RUNNER_ARCH
RUN apk add --no-cache curl tar gzip
RUN mkdir -p /opt/runner && \
    curl -fsSL \
      "https://github.com/actions/runner/releases/download/v${RUNNER_VERSION}/actions-runner-linux-${RUNNER_ARCH}-${RUNNER_VERSION}.tar.gz" \
      | tar xz -C /opt/runner

# Stage 2: minimal runtime image. dotnet/runtime-deps is the Microsoft-
# blessed base for .NET 8 apps — has exactly the libraries actions/
# runner needs and nothing else.
FROM mcr.microsoft.com/dotnet/runtime-deps:8.0
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
      ca-certificates curl git jq sudo libicu-dev \
    && rm -rf /var/lib/apt/lists/*
COPY --from=fetch /opt/runner /opt/runner
RUN useradd -m -u 1001 runner && \
    chown -R runner:runner /opt/runner
USER runner
WORKDIR /opt/runner

# DOCKER_HOST default for cloudrun (cells 5+7); per-execution env can
# override (the gcf variant uses :3376; set in the Cloud Run spec, not
# the dispatcher).
ENV DOCKER_HOST=tcp://localhost:3375

# The dispatcher's per-execution env override sets RUNNER_REG_TOKEN,
# RUNNER_NAME, RUNNER_LABELS, RUNNER_REPO and overrides nothing else.
# This entrypoint is the standard configure+run flow that the upstream
# actions/runner README documents — no sockerless logic. Stays as a
# small shell wrapper, NOT a custom binary.
ENTRYPOINT ["/bin/bash", "-c", "./config.sh --unattended --replace --ephemeral --url \"https://github.com/${RUNNER_REPO}\" --token \"${RUNNER_REG_TOKEN}\" --name \"${RUNNER_NAME}\" --labels \"${RUNNER_LABELS}\" && ./run.sh --once"]
