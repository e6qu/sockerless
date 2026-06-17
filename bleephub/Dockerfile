FROM golang:1.25 AS builder

WORKDIR /src

# --- Build the cloud simulators (job containers run as sim cloud tasks) ---
# One image serves multiple backends; run-integration.sh picks via
# BLEEPHUB_BACKEND (ecs → AWS sim, aca → Azure sim, …).
COPY simulators/realexec/ /realexec/
COPY simulators/aws/ /sim-aws/
RUN cd /sim-aws && GOWORK=off go build -tags noui -o /out/simulator-aws .
COPY simulators/azure/ /sim-azure/
RUN cd /sim-azure && GOWORK=off go build -tags noui -o /out/simulator-azure .
COPY simulators/gcp/ /sim-gcp/
RUN cd /sim-gcp && GOWORK=off go build -tags noui -o /out/simulator-gcp .

# --- Build the backends + agent (the runner's DOCKER_HOST target) ---
COPY api/ api/
COPY agent/ agent/
COPY backends/core/ backends/core/
COPY backends/aws-common/ backends/aws-common/
COPY backends/ecs/ backends/ecs/
RUN cd backends/ecs && GOWORK=off go mod download && \
    GOWORK=off go build -tags noui -o /out/sockerless-backend-ecs ./cmd/sockerless-backend-ecs
COPY backends/azure-common/ backends/azure-common/
COPY backends/aca/ backends/aca/
RUN cd backends/aca && GOWORK=off go mod download && \
    GOWORK=off go build -tags noui -o /out/sockerless-backend-aca ./cmd/sockerless-backend-aca
COPY backends/azure-functions/ backends/azure-functions/
RUN cd backends/azure-functions && GOWORK=off go mod download && \
    GOWORK=off go build -tags noui -o /out/sockerless-backend-azf ./cmd/sockerless-backend-azf
COPY backends/gcp-common/ backends/gcp-common/
COPY backends/cloudrun/ backends/cloudrun/
RUN cd backends/cloudrun && GOWORK=off go mod download && \
    GOWORK=off go build -tags noui -o /out/sockerless-backend-cloudrun ./cmd/sockerless-backend-cloudrun
COPY backends/cloudrun-functions/ backends/cloudrun-functions/
RUN cd backends/cloudrun-functions && GOWORK=off go mod download && \
    GOWORK=off go build -tags noui -o /out/sockerless-backend-gcf ./cmd/sockerless-backend-gcf
# CGO_ENABLED=0: the agent and the reverse-agent bootstrap are injected
# into arbitrary workload base images (alpine/musl, distroless, scratch),
# so they must be statically linked — a glibc-dynamic binary fails to exec
# on musl with "No such file or directory" (the missing dynamic loader).
RUN cd agent && GOWORK=off go mod download && \
    CGO_ENABLED=0 GOWORK=off go build -o /out/sockerless-agent ./cmd/sockerless-agent && \
    CGO_ENABLED=0 GOWORK=off go build -o /out/sockerless-cloudrun-bootstrap ./cmd/sockerless-cloudrun-bootstrap && \
    CGO_ENABLED=0 GOWORK=off go build -o /out/sockerless-gcf-bootstrap ./cmd/sockerless-gcf-bootstrap && \
    CGO_ENABLED=0 GOWORK=off go build -o /out/sockerless-azf-bootstrap ./cmd/sockerless-azf-bootstrap

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
# openssl: the cloudrun cell generates a service-account RSA key for the
# fake SA JSON (token_uri → sim /token) the backend's google clients sign.
RUN apt-get update && apt-get install -y --no-install-recommends \
    curl ca-certificates jq git libicu70 libssl3 openssl python3 python3-nacl docker.io && \
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
