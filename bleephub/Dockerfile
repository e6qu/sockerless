FROM golang:1.25 AS builder

WORKDIR /src

# --- Build the AWS simulator (job containers run as sim-ECS tasks) ---
COPY simulators/realexec/ /realexec/
COPY simulators/aws/ /sim-aws/
RUN cd /sim-aws && GOWORK=off go build -tags noui -o /out/simulator-aws .

# --- Build the ECS backend + agent (the runner's DOCKER_HOST target) ---
COPY api/ api/
COPY agent/ agent/
COPY backends/core/ backends/core/
COPY backends/aws-common/ backends/aws-common/
COPY backends/ecs/ backends/ecs/
RUN cd backends/ecs && GOWORK=off go mod download && \
    GOWORK=off go build -tags noui -o /out/sockerless-backend-ecs ./cmd/sockerless-backend-ecs
RUN cd agent && GOWORK=off go mod download && \
    GOWORK=off go build -o /out/sockerless-agent ./cmd/sockerless-agent

# --- Build the dispatcher (runner-as-task control plane) ---
COPY github-runner-dispatcher-aws/ github-runner-dispatcher-aws/
RUN cd github-runner-dispatcher-aws && GOWORK=off go mod download && \
    GOWORK=off go build -o /out/github-runner-dispatcher-aws ./cmd/github-runner-dispatcher-aws

# --- Build bleephub (last: the hottest layer while iterating) ---
COPY bleephub/ bleephub/
# noui: the runner harness exercises the protocol surface; the UI dist
# is a gitignored build artifact that doesn't exist in CI checkouts.
RUN cd bleephub && GOWORK=off go build -tags noui -o /out/bleephub ./cmd

# --- Runtime stage ---
FROM ubuntu:22.04

# python3-nacl: the harness seals secrets with the real libsodium
# sealed-box contract before PUTting them.
# docker.io: client CLI for the dispatcher's `docker run` shell-outs
# and the harness's spawn-image build via the mounted host socket.
RUN apt-get update && apt-get install -y --no-install-recommends \
    curl ca-certificates jq git libicu70 libssl3 python3 python3-nacl docker.io && \
    rm -rf /var/lib/apt/lists/*

# Download official GitHub Actions runner (auto-detect architecture)
ARG RUNNER_VERSION=2.321.0
RUN mkdir -p /runner && \
    ARCH=$(dpkg --print-architecture) && \
    case "$ARCH" in \
        amd64) RUNNER_ARCH=x64 ;; \
        arm64) RUNNER_ARCH=arm64 ;; \
        *) echo "Unsupported arch: $ARCH"; exit 1 ;; \
    esac && \
    curl -fsSL "https://github.com/actions/runner/releases/download/v${RUNNER_VERSION}/actions-runner-linux-${RUNNER_ARCH}-${RUNNER_VERSION}.tar.gz" \
    | tar xz -C /runner

COPY --from=builder /out/ /usr/local/bin/
COPY bleephub/test/ /test/
RUN chmod +x /test/*.sh

WORKDIR /test
# bash-form: /test may be a runtime bind mount (local iteration) whose
# exec bit doesn't survive every host filesystem (macOS virtiofs).
ENTRYPOINT ["/bin/bash", "/test/run-integration.sh"]
