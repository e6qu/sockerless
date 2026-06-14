FROM golang:1.25 AS builder

WORKDIR /src

# --- Cloud simulator (ECS jobs run as sim ECS tasks on the host engine) ---
COPY simulators/realexec/ /realexec/
COPY simulators/aws/ /sim-aws/
RUN cd /sim-aws && GOWORK=off go build -tags noui -o /out/simulator-aws .

# --- Backend (the gitlab-runner docker executor's DOCKER_HOST target) + agent ---
COPY api/ api/
COPY agent/ agent/
COPY backends/core/ backends/core/
COPY backends/aws-common/ backends/aws-common/
COPY backends/ecs/ backends/ecs/
RUN cd backends/ecs && GOWORK=off go mod download && \
    GOWORK=off go build -tags noui -o /out/sockerless-backend-ecs ./cmd/sockerless-backend-ecs
# CGO_ENABLED=0: the reverse-agent is injected into arbitrary workload images,
# so it must be statically linked.
RUN cd agent && GOWORK=off go mod download && \
    CGO_ENABLED=0 GOWORK=off go build -o /out/sockerless-agent ./cmd/sockerless-agent

# --- bleeplab GitLab control-plane simulator ---
COPY bleeplab/ bleeplab/
RUN cd bleeplab && GOWORK=off go build -o /out/bleeplab ./cmd

# --- Runtime stage ---
FROM ubuntu:22.04

RUN apt-get update && apt-get install -y --no-install-recommends \
    curl ca-certificates jq git docker.io && \
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
