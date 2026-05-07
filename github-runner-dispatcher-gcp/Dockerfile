# Multi-stage build of github-runner-dispatcher-gcp for Cloud Run.
# Build context is the repo root (workspaces include the AWS module
# via a `replace` directive, so we need both modules in scope).
#
# Build:
#   docker buildx build --platform linux/amd64 \
#     -f github-runner-dispatcher-gcp/Dockerfile \
#     -t us-central1-docker.pkg.dev/<proj>/<repo>/dispatcher:gcp-amd64 \
#     --push .
#
# Run on Cloud Run:
#   cloud run deploy with REPO=<owner/repo>, GITHUB_TOKEN secret,
#   ADC via runner SA. Container binds $PORT for /healthz; polling
#   loop runs in goroutine.

FROM golang:1.25-alpine AS build
WORKDIR /src
RUN apk add --no-cache git
# Copy both modules — gcp module replaces aws via local path.
COPY github-runner-dispatcher-aws/ github-runner-dispatcher-aws/
COPY github-runner-dispatcher-gcp/ github-runner-dispatcher-gcp/
WORKDIR /src/github-runner-dispatcher-gcp
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' \
    -o /out/dispatcher ./cmd/github-runner-dispatcher-gcp

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/dispatcher /dispatcher
COPY github-runner-dispatcher-gcp/config.toml.example /etc/sockerless-dispatcher/config.toml
USER nonroot:nonroot
ENV PORT=8080
ENTRYPOINT ["/dispatcher", "--config", "/etc/sockerless-dispatcher/config.toml"]
