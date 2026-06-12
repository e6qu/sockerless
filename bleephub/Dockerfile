FROM golang:1.25 AS builder

WORKDIR /src

# --- Build bleephub ---
COPY bleephub/ bleephub/
# noui: the runner harness exercises the protocol surface; the UI dist
# is a gitignored build artifact that doesn't exist in CI checkouts.
RUN cd bleephub && GOWORK=off go build -tags noui -o /out/bleephub ./cmd

# --- Runtime stage ---
FROM ubuntu:22.04

# python3-nacl: the harness seals secrets with the real libsodium
# sealed-box contract before PUTting them.
RUN apt-get update && apt-get install -y --no-install-recommends \
    curl ca-certificates jq git libicu70 libssl3 python3 python3-nacl && \
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
ENTRYPOINT ["/test/run-integration.sh"]
