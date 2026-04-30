# GitHub Actions runner image for sockerless ECS dispatch.
#
# Single-container architecture: this image contains both the
# `actions/runner` binary AND `sockerless-backend-ecs`. The entrypoint
# starts sockerless on `localhost:3375`, then the runner with
# `DOCKER_HOST=tcp://localhost:3375` — so the runner's `docker
# create / docker exec` calls (including the `container:`-directive
# bind mounts of `/home/runner/_work`) flow through sockerless, which
# translates them to ECS RunTask + EFS-shared workspace volumes.
#
# Why single-container vs sidecar: the user originally picked sidecar
# but single-container has the same architectural correctness (both
# binaries Linux, both see the same EFS workspace, both share
# localhost) with a simpler image-management story (one image, one
# build, one push). Sidecar can be a follow-up if multi-image
# isolation is desired.
#
# Build: this Dockerfile expects `sockerless-backend-ecs` (linux/amd64)
# pre-built next to the Dockerfile (cross-compiled from the host).
# Image is linux/amd64 because Fargate's default RuntimePlatform.CpuArchitecture
# is X86_64 — change ARG TARGETARCH if you switch to ARM64 Fargate.

FROM ubuntu:24.04

ARG RUNNER_VERSION=2.334.0
# Asset architecture for the actions/runner binary. Match this to
# the ECS RuntimePlatform.CpuArchitecture: X86_64 → x64, ARM64 → arm64.
# Use a non-standard ARG name (not TARGETARCH) so podman's --platform
# flag doesn't override it with `amd64` / `arm64`.
ARG RUNNER_ARCH=x64
# Static docker CLI version. The runner uses `docker` to honor a
# workflow's `container:` directive and any `run: docker run …`
# steps. The CLI is pure-client; it talks to localhost:3375 (sockerless)
# over TCP — no docker daemon, no docker.sock.
ARG DOCKER_VERSION=27.3.1
# Static docker CLI architecture (uname -m form): x64 → x86_64, arm64 → aarch64.
ARG DOCKER_ARCH=x86_64

RUN apt-get update && apt-get install -y --no-install-recommends \
      curl ca-certificates libicu74 git sudo jq \
      iproute2 && \
    rm -rf /var/lib/apt/lists/*
# iproute2 provides `ip` — sockerless's LinuxNetworkDriver invokes
# `ip netns add` / `ip link add` to create real Linux network
# namespaces and veth pairs alongside the cloud-side VPC security
# group + Cloud Map namespace that the ECS NetworkCreate wraps.

RUN curl -fsSL "https://download.docker.com/linux/static/stable/${DOCKER_ARCH}/docker-${DOCKER_VERSION}.tgz" \
    | tar xz --strip-components=1 -C /usr/local/bin docker/docker && \
    /usr/local/bin/docker --version

# Pre-built sockerless-backend-ecs binary (built outside the image
# via `GOOS=linux GOARCH=amd64 go build`). Living next to the
# Dockerfile keeps the build context small.
COPY sockerless-backend-ecs /usr/local/bin/sockerless-backend-ecs
RUN chmod +x /usr/local/bin/sockerless-backend-ecs

RUN useradd -m runner && \
    echo "runner ALL=(ALL) NOPASSWD:ALL" >> /etc/sudoers

USER runner
WORKDIR /home/runner

RUN curl -fsSL \
      "https://github.com/actions/runner/releases/download/v${RUNNER_VERSION}/actions-runner-linux-${RUNNER_ARCH}-${RUNNER_VERSION}.tar.gz" \
    | tar xz

# Stage the externals tree to a non-mount location so the runner-task's
# entrypoint can rsync it into the EFS-mounted /home/runner/externals on
# startup. Without this, the EFS mount at /home/runner/externals would
# hide the actions/runner-shipped node20/ directory (Linux mount semantics:
# the EFS access point becomes the directory's contents, replacing whatever
# the image had there).
RUN cp -a /home/runner/externals /home/runner/externals.staged \
    && rm -rf /home/runner/externals \
    && mkdir -p /home/runner/externals \
    && chown -R runner:runner /home/runner/externals.staged /home/runner/externals

COPY --chown=runner:runner entrypoint-ecs.sh /home/runner/entrypoint.sh

# Sockerless ECS backend recognises the runner-task workload by image
# label and uses the pre-registered task definition (not implemented
# yet — Phase 110b follow-up; for now the harness calls ECS RunTask
# directly with the runner-task task-definition ARN).
LABEL com.sockerless.ecs.task-definition-family="sockerless-runner"

ENTRYPOINT ["/home/runner/entrypoint.sh"]
