# backend-cloudrun

Google Cloud Run backend. Maps Docker container operations to Cloud Run Jobs and Executions.

## Resource mapping

| Docker concept | Cloud Run resource |
|---------------|-------------------|
| Container create | _(registers in store)_ |
| Container start | `CreateJob` + `RunJob` (creates Execution) |
| Container stop/kill | Cancel Execution |
| Container remove | `DeleteJob` |
| Container logs | Cloud Logging via Log Admin API |

Jobs are created at start time (not create time) to support restarts cleanly — the old job is deleted before creating a new one.

## Agent mode

Uses **forward agent** by default: after starting a job execution, the backend polls for the execution to reach RUNNING state, extracts the agent address, and dials in.

Also supports **reverse agent** via `SOCKERLESS_CALLBACK_URL`.

## Building

```sh
cd backends/cloudrun
go build -o sockerless-backend-cloudrun ./cmd/sockerless-backend-cloudrun
```

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `SOCKERLESS_GCR_PROJECT` | _(required)_ | GCP project ID |
| `SOCKERLESS_GCR_REGION` | `us-central1` | GCP region |
| `SOCKERLESS_GCR_VPC_CONNECTOR` | | VPC connector for network access |
| `SOCKERLESS_GCR_LOG_ID` | `sockerless` | Cloud Logging log ID |
| `SOCKERLESS_GCR_AGENT_IMAGE` | `sockerless/agent:latest` | Sidecar agent image |
| `SOCKERLESS_GCR_AGENT_TOKEN` | | Default agent authentication token |
| `SOCKERLESS_CALLBACK_URL` | | Backend URL for reverse agent mode |
| `SOCKERLESS_ENDPOINT_URL` | | Custom GCP endpoint (simulator mode) |

### Terraform outputs

The `terraform/modules/cloudrun` module produces these outputs. Use `terragrunt output` from `terraform/environments/cloudrun/live` to extract them.

| Terraform Output | Environment Variable |
|---|---|
| `project_id` | `SOCKERLESS_GCR_PROJECT` |
| `region` | `SOCKERLESS_GCR_REGION` |
| `vpc_connector_name` | `SOCKERLESS_GCR_VPC_CONNECTOR` |
| `artifact_registry_repository_url` | `SOCKERLESS_GCR_AGENT_IMAGE` (after building and pushing) |

## Project structure

```
cloudrun/
├── cmd/sockerless-backend-cloudrun/
│   └── main.go          CLI entrypoint
├── server.go            Server type, route overrides
├── config.go            Config struct, env parsing, validation
├── gcp.go               GCP SDK client initialization
├── containers.go        Create, start, stop, kill, remove handlers
├── jobspec.go           Cloud Run Job protobuf builder
├── logs.go              Cloud Logging streaming
├── images.go            Image pull handler
├── extended.go          Pause, unpause, restart, volume prune
├── store.go             CloudRunState type
├── registry.go          Container image registry support
└── errors.go            GCP error mapping
```

## Example deployment

See [examples/terraform/](examples/terraform/) for a complete Terraform example that provisions the GCP infrastructure (VPC, Cloud Run APIs, Artifact Registry, service account) and walks through running Docker commands against Cloud Run Jobs.

## Docker API mapping

For a detailed breakdown of how each Docker REST API endpoint and CLI command maps to Google Cloud Run Jobs operations — including what's supported, what's not, and how it compares to vanilla Docker — see [docs/docker_api_mapping.md](docs/docker_api_mapping.md).

## Testing

```sh
make sim-test-gcp    # simulator integration tests
make docker-test     # Docker-based full test
```
