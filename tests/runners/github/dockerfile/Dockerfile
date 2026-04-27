# GitHub Actions runner in a Linux container.
#
# Why a Linux container: GitHub Actions' `container:` directive (the
# canonical pattern for jobs that need a specific image, e.g. `alpine`,
# `node:20`) only works when the runner itself is on Linux. A
# darwin-native runner refuses with "Container operations are only
# supported on Linux runners". Sockerless's host arch is irrelevant
# (it's the docker server side, the runner is just a docker client) —
# but the runner *process* needs Linux. Solved by running the runner
# inside a Linux container locally.
#
# Network model: the container talks to the sockerless daemon on the
# host via DOCKER_HOST=tcp://host.docker.internal:<port>. Sockerless
# dispatches each job container to ECS Fargate or Lambda per its
# backend. So three layers of containers in flight simultaneously:
#   (1) this runner container (long-lived during the test cell)
#   (2) per-job containers spawned by the runner (short-lived)
#       — these are the ones sockerless dispatches to the cloud
FROM ubuntu:24.04

ARG RUNNER_VERSION=2.334.0
# Architecture of the runner binary itself — should match the laptop's
# linux-VM-arch (Docker Desktop on Apple Silicon = linux/arm64; on
# Intel = linux/amd64). Override via --build-arg if the host VM diverges.
ARG TARGETARCH=arm64

RUN apt-get update && apt-get install -y --no-install-recommends \
      curl ca-certificates libicu74 git sudo jq && \
    rm -rf /var/lib/apt/lists/*

RUN useradd -m runner && \
    echo "runner ALL=(ALL) NOPASSWD:ALL" >> /etc/sudoers

USER runner
WORKDIR /home/runner

RUN curl -fsSL \
      "https://github.com/actions/runner/releases/download/v${RUNNER_VERSION}/actions-runner-linux-${TARGETARCH}-${RUNNER_VERSION}.tar.gz" \
    | tar xz

COPY --chown=runner:runner entrypoint.sh /home/runner/entrypoint.sh

ENTRYPOINT ["/home/runner/entrypoint.sh"]
