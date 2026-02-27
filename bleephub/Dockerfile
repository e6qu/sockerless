FROM golang:1.25 AS builder

WORKDIR /src

# --- Build bleephub ---
COPY bleephub/ bleephub/
RUN cd bleephub && GOWORK=off go build -o /out/bleephub ./cmd

# --- Build memory backend ---
COPY api/ api/
COPY agent/ agent/
COPY sandbox/ sandbox/
COPY backends/core/ backends/core/
COPY backends/memory/ backends/memory/
RUN cd backends/memory && GOWORK=off go mod download && \
    GOWORK=off go build -o /out/sockerless-backend-memory ./cmd

# --- Build Docker frontend ---
COPY frontends/docker/ frontends/docker/
RUN cd frontends/docker && GOWORK=off go mod download && \
    GOWORK=off go build -o /out/sockerless-frontend-docker ./cmd

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
