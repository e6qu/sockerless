FROM golang:1.25 AS builder

WORKDIR /src

# --- Build bleephub ---
COPY bleephub/ bleephub/
RUN cd bleephub && GOWORK=off go build -o /out/bleephub ./cmd

# --- Build ECS backend (for integration testing) ---
COPY api/ api/
COPY agent/ agent/
COPY backends/core/ backends/core/
COPY backends/ecs/ backends/ecs/
RUN cd backends/ecs && GOWORK=off go mod download && \
    GOWORK=off go build -tags noui -o /out/sockerless-backend-ecs ./cmd/sockerless-backend-ecs

# --- Build AWS simulator ---
COPY simulators/aws/ /sim-aws/
RUN cd /sim-aws && GOWORK=off go build -tags noui -o /out/simulator-aws .

# --- Runtime stage ---
FROM ubuntu:22.04

RUN apt-get update && apt-get install -y --no-install-recommends \
    curl ca-certificates jq libicu70 libssl3 && \
    rm -rf /var/lib/apt/lists/*

# Install Docker CLI (static binary) — the runner needs it to manage containers via DOCKER_HOST
RUN curl -fsSL "https://download.docker.com/linux/static/stable/$(uname -m)/docker-27.5.1.tgz" \
    | tar xz --strip-components=1 -C /usr/local/bin docker/docker

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
ENTRYPOINT ["/test/run-integration.sh"]
