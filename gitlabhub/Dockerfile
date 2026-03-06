FROM golang:1.25 AS builder

WORKDIR /src

# Copy all modules needed for building
COPY api/ ./api/
COPY agent/ ./agent/
COPY backends/core/ ./backends/core/
COPY gitlabhub/ ./gitlabhub/
COPY go.work ./

# Build gitlabhub
WORKDIR /src/gitlabhub
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /usr/local/bin/gitlabhub ./cmd

# Runtime image with gitlab-runner
FROM gitlab/gitlab-runner:latest

RUN apt-get update && apt-get install -y --no-install-recommends \
    curl jq && \
    rm -rf /var/lib/apt/lists/*

# Copy built binaries
COPY --from=builder /usr/local/bin/gitlabhub /usr/local/bin/

# Copy test scripts
COPY gitlabhub/test/ /test/

# Create runner working directory
RUN mkdir -p /home/gitlab-runner/builds

ENTRYPOINT ["/test/run-integration.sh"]
