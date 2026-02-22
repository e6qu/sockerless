# backend-gcf

Google Cloud Run Functions (2nd gen) backend. Maps Docker container operations to Cloud Functions with Docker runtime.

## Resource mapping

| Docker concept | Cloud Functions resource |
|---------------|------------------------|
| Container create | `CreateFunction` (Docker runtime) |
| Container start | HTTP POST invoke (async) |
| Container stop | No-op (runs to completion) |
| Container kill | Disconnects reverse agent |
| Container remove | `DeleteFunction` |
| Container logs | Cloud Logging via Log Admin API |

## Agent mode

Uses **reverse agent** exclusively. Cloud Functions cannot accept inbound connections, so the agent inside the function dials back to the backend via `SOCKERLESS_CALLBACK_URL`.

Helper and cache containers auto-stop after 500ms.

## Building

```sh
cd backends/cloudrun-functions
go build -o sockerless-backend-gcf ./cmd/sockerless-backend-gcf
```

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `SOCKERLESS_GCF_PROJECT` | _(required)_ | GCP project ID |
| `SOCKERLESS_GCF_REGION` | `us-central1` | GCP region |
| `SOCKERLESS_GCF_SERVICE_ACCOUNT` | | Service account for functions |
| `SOCKERLESS_GCF_TIMEOUT` | `3600` | Function timeout (seconds) |
| `SOCKERLESS_GCF_MEMORY` | `1Gi` | Function memory limit |
| `SOCKERLESS_GCF_CPU` | `1` | Function CPU allocation |
| `SOCKERLESS_CALLBACK_URL` | | Backend URL for reverse agent connections |
| `SOCKERLESS_ENDPOINT_URL` | | Custom GCP endpoint (simulator mode) |

## Project structure

```
cloudrun-functions/
├── cmd/sockerless-backend-gcf/
│   └── main.go          CLI entrypoint
├── server.go            Server type, route overrides
├── config.go            Config struct, env parsing, validation
├── gcp.go               GCP SDK client initialization
├── containers.go        Create, start, stop, kill, remove handlers
├── logs.go              Cloud Logging streaming
├── images.go            Image pull/load handlers
├── extended.go          Restart, prune
├── store.go             GCFState type
└── errors.go            GCP error mapping
```

## Example deployment

See [examples/terraform/](examples/terraform/) for a complete Terraform example that provisions the GCP infrastructure (Cloud Functions APIs, Artifact Registry, service account) and walks through running Docker commands against Cloud Run Functions.

## Docker API mapping

For a detailed breakdown of how each Docker REST API endpoint and CLI command maps to Google Cloud Functions operations — including what's supported, what's not, and how it compares to vanilla Docker — see [docs/docker_api_mapping.md](docs/docker_api_mapping.md).

## Testing

```sh
make sim-test-gcp    # simulator integration tests
make docker-test     # Docker-based full test
```
