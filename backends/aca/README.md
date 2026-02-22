# backend-aca

Azure Container Apps backend. Maps Docker container operations to Container Apps Jobs and Executions.

## Resource mapping

| Docker concept | Azure resource |
|---------------|---------------|
| Container create | _(registers in store)_ |
| Container start | Create Container App Job + Start Execution |
| Container stop/kill | Stop Execution |
| Container remove | Delete Job |
| Container logs | Azure Monitor Log Analytics query |

Jobs are created at start time to support clean restarts.

## Agent mode

Uses **forward agent** by default: after starting a job execution, the backend polls for RUNNING state, extracts the agent address, and dials in.

Also supports **reverse agent** via `SOCKERLESS_CALLBACK_URL`.

## Building

```sh
cd backends/aca
go build -o sockerless-backend-aca ./cmd/sockerless-backend-aca
```

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `SOCKERLESS_ACA_SUBSCRIPTION_ID` | _(required)_ | Azure subscription ID |
| `SOCKERLESS_ACA_RESOURCE_GROUP` | _(required)_ | Azure resource group |
| `SOCKERLESS_ACA_ENVIRONMENT` | `sockerless` | Container Apps Environment name |
| `SOCKERLESS_ACA_LOCATION` | `eastus` | Azure region |
| `SOCKERLESS_ACA_LOG_ANALYTICS_WORKSPACE` | | Log Analytics workspace ID |
| `SOCKERLESS_ACA_STORAGE_ACCOUNT` | | Storage account for volumes |
| `SOCKERLESS_ACA_AGENT_IMAGE` | `sockerless/agent:latest` | Sidecar agent image |
| `SOCKERLESS_ACA_AGENT_TOKEN` | | Default agent authentication token |
| `SOCKERLESS_CALLBACK_URL` | | Backend URL for reverse agent mode |
| `SOCKERLESS_ENDPOINT_URL` | | Custom Azure endpoint (simulator mode) |

## Project structure

```
aca/
├── cmd/sockerless-backend-aca/
│   └── main.go          CLI entrypoint
├── server.go            Server type, route overrides
├── config.go            Config struct, env parsing, validation
├── azure.go             Azure SDK client initialization
├── containers.go        Create, start, stop, kill, remove handlers
├── jobspec.go           Container Apps Job spec builder
├── logs.go              Azure Monitor Log Analytics streaming
├── images.go            Image pull handler
├── extended.go          Pause, unpause, restart, volume prune
├── store.go             ACAState type
├── registry.go          Container image registry support
└── errors.go            Azure error mapping
```

## Example deployment

See [examples/terraform/](examples/terraform/) for a complete Terraform example that provisions the Azure infrastructure (Resource Group, Container Apps Environment, VNet, Log Analytics, ACR, Storage) and walks through running Docker commands against Container Apps Jobs.

## Docker API mapping

For a detailed breakdown of how each Docker REST API endpoint and CLI command maps to Azure Container Apps operations — including what's supported, what's not, and how it compares to vanilla Docker — see [docs/docker_api_mapping.md](docs/docker_api_mapping.md).

## Testing

```sh
make sim-test-azure  # simulator integration tests
make docker-test     # Docker-based full test
```
