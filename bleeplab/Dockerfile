FROM golang:1.25 AS builder

WORKDIR /src

# --- Cloud simulators (one image serves multiple backends; the harness picks
# via BLEEPLAB_BACKEND: ecs → AWS sim, cloudrun → GCP sim) ---
COPY simulators/realexec/ /realexec/
COPY simulators/aws/ /sim-aws/
RUN cd /sim-aws && GOWORK=off go build -tags noui -o /out/simulator-aws .
COPY simulators/gcp/ /sim-gcp/
RUN cd /sim-gcp && GOWORK=off go build -tags noui -o /out/simulator-gcp .
COPY simulators/azure/ /sim-azure/
RUN cd /sim-azure && GOWORK=off go build -tags noui -o /out/simulator-azure .

# --- Backends (the gitlab-runner docker executor's DOCKER_HOST target) + agent ---
COPY api/ api/
COPY agent/ agent/
COPY backends/core/ backends/core/
COPY backends/aws-common/ backends/aws-common/
COPY backends/ecs/ backends/ecs/
RUN cd backends/ecs && GOWORK=off go mod download && \
    GOWORK=off go build -tags noui -o /out/sockerless-backend-ecs ./cmd/sockerless-backend-ecs
COPY backends/gcp-common/ backends/gcp-common/
COPY backends/cloudrun/ backends/cloudrun/
RUN cd backends/cloudrun && GOWORK=off go mod download && \
    GOWORK=off go build -tags noui -o /out/sockerless-backend-cloudrun ./cmd/sockerless-backend-cloudrun
COPY backends/cloudrun-functions/ backends/cloudrun-functions/
RUN cd backends/cloudrun-functions && GOWORK=off go mod download && \
    GOWORK=off go build -tags noui -o /out/sockerless-backend-gcf ./cmd/sockerless-backend-gcf
COPY backends/azure-common/ backends/azure-common/
COPY backends/aca/ backends/aca/
RUN cd backends/aca && GOWORK=off go mod download && \
    GOWORK=off go build -tags noui -o /out/sockerless-backend-aca ./cmd/sockerless-backend-aca
COPY backends/azure-functions/ backends/azure-functions/
RUN cd backends/azure-functions && GOWORK=off go mod download && \
    GOWORK=off go build -tags noui -o /out/sockerless-backend-azf ./cmd/sockerless-backend-azf
# CGO_ENABLED=0: the reverse-agent + cloudrun/gcf/azf bootstraps are injected
# into arbitrary workload images, so they must be statically linked. The
# cloudrun bootstrap is reused as the ACA App overlay bootstrap.
RUN cd agent && GOWORK=off go mod download && \
    CGO_ENABLED=0 GOWORK=off go build -o /out/sockerless-agent ./cmd/sockerless-agent && \
    CGO_ENABLED=0 GOWORK=off go build -o /out/sockerless-cloudrun-bootstrap ./cmd/sockerless-cloudrun-bootstrap && \
    CGO_ENABLED=0 GOWORK=off go build -o /out/sockerless-gcf-bootstrap ./cmd/sockerless-gcf-bootstrap && \
    CGO_ENABLED=0 GOWORK=off go build -o /out/sockerless-azf-bootstrap ./cmd/sockerless-azf-bootstrap

# --- bleeplab GitLab control-plane simulator ---
# Built with -tags noui: the runner harness is headless and the embedded SPA's
# dist/ isn't in the build context (it's a generated, gitignored artifact).
COPY bleeplab/ bleeplab/
RUN cd bleeplab && GOWORK=off go build -tags noui -o /out/bleeplab ./cmd

# --- Runtime stage ---
FROM ubuntu:22.04

# openssl: the cloudrun cell generates a service-account RSA key for the fake
# SA JSON (token_uri → sim /token) the backend's google clients sign.
RUN apt-get update && apt-get install -y --no-install-recommends \
    curl ca-certificates jq git docker.io openssl && \
    rm -rf /var/lib/apt/lists/*

# Official gitlab-runner binary (arch-aware) — the real docker-executor runner
# that registers against bleeplab and dispatches jobs through sockerless.
ARG GITLAB_RUNNER_VERSION=v18.11.3
RUN ARCH=$(dpkg --print-architecture) && \
    case "$ARCH" in \
        amd64) RUNNER_ARCH=amd64 ;; \
        arm64) RUNNER_ARCH=arm64 ;; \
        *) echo "Unsupported arch: $ARCH"; exit 1 ;; \
    esac && \
    curl -fsSL "https://gitlab-runner-downloads.s3.amazonaws.com/${GITLAB_RUNNER_VERSION}/binaries/gitlab-runner-linux-${RUNNER_ARCH}" \
        -o /usr/local/bin/gitlab-runner && \
    chmod +x /usr/local/bin/gitlab-runner

COPY --from=builder /out/ /usr/local/bin/
COPY bleeplab/test/ /test/
RUN chmod +x /test/*.sh

WORKDIR /test
ENTRYPOINT ["/bin/bash", "/test/run-integration.sh"]
