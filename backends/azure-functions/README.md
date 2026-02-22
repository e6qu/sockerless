# backend-azf

Azure Functions backend. Maps Docker container operations to Function Apps with custom container images.

## Resource mapping

| Docker concept | Azure resource |
|---------------|---------------|
| Container create | Create App Service Plan + Function App |
| Container start | Start Function App |
| Container stop/kill | Stop Function App |
| Container remove | Delete Function App + App Service Plan |
| Container logs | Azure Monitor Log Analytics query |

## Agent mode

Uses **reverse agent** exclusively. Azure Functions cannot accept arbitrary inbound connections, so the agent inside the function dials back to the backend via `SOCKERLESS_CALLBACK_URL`.

Helper and cache containers auto-stop after 500ms.

## Building

```sh
cd backends/azure-functions
go build -o sockerless-backend-azf ./cmd/sockerless-backend-azf
```

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `SOCKERLESS_AZF_SUBSCRIPTION_ID` | _(required)_ | Azure subscription ID |
| `SOCKERLESS_AZF_RESOURCE_GROUP` | _(required)_ | Azure resource group |
| `SOCKERLESS_AZF_LOCATION` | `eastus` | Azure region |
| `SOCKERLESS_AZF_STORAGE_ACCOUNT` | _(required)_ | Storage account for the function |
| `SOCKERLESS_AZF_REGISTRY` | | Container registry URL |
| `SOCKERLESS_AZF_APP_SERVICE_PLAN` | | App Service Plan name |
| `SOCKERLESS_AZF_TIMEOUT` | `600` | Function timeout (seconds) |
| `SOCKERLESS_AZF_LOG_ANALYTICS_WORKSPACE` | | Log Analytics workspace ID |
| `SOCKERLESS_CALLBACK_URL` | | Backend URL for reverse agent connections |
| `SOCKERLESS_ENDPOINT_URL` | | Custom Azure endpoint (simulator mode) |

### Terraform outputs

The `terraform/modules/azf` module produces these outputs. Use `terragrunt output` from `terraform/environments/azf/live` to extract them.

| Terraform Output | Environment Variable |
|---|---|
| `resource_group_name` | `SOCKERLESS_AZF_RESOURCE_GROUP` |
| `location` | `SOCKERLESS_AZF_LOCATION` |
| `storage_account_name` | `SOCKERLESS_AZF_STORAGE_ACCOUNT` |
| `acr_login_server` | `SOCKERLESS_AZF_REGISTRY` |
| `app_service_plan_id` | `SOCKERLESS_AZF_APP_SERVICE_PLAN` |
| `log_analytics_workspace_id` | `SOCKERLESS_AZF_LOG_ANALYTICS_WORKSPACE` |

`SOCKERLESS_AZF_SUBSCRIPTION_ID` is not a terraform output — use `az account show --query id -o tsv`.

## Project structure

```
azure-functions/
├── cmd/sockerless-backend-azf/
│   └── main.go          CLI entrypoint
├── server.go            Server type, route overrides
├── config.go            Config struct, env parsing, validation
├── azure.go             Azure SDK client initialization
├── containers.go        Create, start, stop, kill, remove handlers
├── logs.go              Azure Monitor Log Analytics streaming
├── images.go            Image pull/load handlers
├── extended.go          Restart, prune
├── store.go             AZFState type
└── errors.go            Azure error mapping
```

## Example deployment

See [examples/terraform/](examples/terraform/) for a complete Terraform example that provisions the Azure infrastructure (Resource Group, App Service Plan, Storage Account, Log Analytics, ACR) and walks through running Docker commands against Azure Functions.

## Docker API mapping

For a detailed breakdown of how each Docker REST API endpoint and CLI command maps to Azure Functions operations — including what's supported, what's not, and how it compares to vanilla Docker — see [docs/docker_api_mapping.md](docs/docker_api_mapping.md).

## Testing

```sh
make sim-test-azure  # simulator integration tests
make docker-test     # Docker-based full test
```
