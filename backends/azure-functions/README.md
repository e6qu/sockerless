# Azure Functions Backend

Runs Docker containers as Azure Function Apps with custom container images, with Log Analytics for log streaming. Frontend speaks Docker REST API v1.44; backend speaks the App Service / Function Apps / Log Analytics / ACR APIs.

## Reference adaptors

| Direction | Adaptor | Min version | What it proves |
|---|---|---|---|
| **Frontend (Docker API)** | [Docker Go SDK](https://pkg.go.dev/github.com/docker/docker/client) | v25+ | `docker run` → Function invoke via `tcp://localhost:3375`. |
| | [`docker` CLI](https://docs.docker.com/engine/reference/commandline/cli/) | 29.x | Wire-level [Docker REST API v1.44](https://docs.docker.com/engine/api/v1.44/). |
| **Backend (Azure API)** | [`az` CLI](https://learn.microsoft.com/en-us/cli/azure/functionapp) | 2.60+ | `az functionapp show`, `az monitor app-insights query`. |
| | [Azure SDK for Go](https://pkg.go.dev/github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appservice/armappservice) | v1.6+ | The [App Service ARM REST API](https://learn.microsoft.com/en-us/rest/api/appservice/) (`Sites`) and Application Insights queries the backend issues. |
| | [Terraform `azurerm` provider](https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs/resources/linux_function_app) | v4+ | `azurerm_linux_function_app` with container `image` provisions the function infra. |

Local development replaces the backend-side upstream with [`simulators/azure`](../../simulators/azure/README.md). Container mode only (no native runtimes) — see [`memory/feedback_faas_container_mode.md`](../../).

## Validation

| Test path | What runs | Last green |
|---|---|---|
| `tests/` (Docker SDK against running backend, AZF profile) | Container lifecycle round-trip via Function invoke. | 2026-05-13 |
| `simulators/azure/sdk-tests/` Functions package | App Service ARM calls validated against the sim. | 2026-05-13 |
| `simulators/azure/terraform-tests/` (Docker-only, TLS) | `azurerm_linux_function_app` apply / destroy. | 2026-05-13 |
| `make backends/azure-functions/test` | Leaf-Makefile unit + integration suite. | 2026-05-13 |

## Wiring the adaptor

```bash
cd backends/azure-functions && make build
./sockerless-backend-azure-functions --addr :3375 --log-level info &
export DOCKER_HOST=tcp://localhost:3375
```

### Config (config.yaml)

```yaml
environments:
  my-azf:
    backend: azf
    addr: ":3375"
    log_level: info
    azure:
      subscription_id: a1b2c3d4-e5f6-7890-abcd-ef1234567890
      azf:
        resource_group: sockerless-rg
        location: eastus
        storage_account: sockerlessstorage
        registry: sockerless.azurecr.io
        app_service_plan: sockerless-plan
        timeout: 600
        log_analytics_workspace: /subscriptions/.../workspaces/sockerless-logs
    common:
      callback_url: https://backend.example.com
      poll_interval: 2s
      agent_timeout: 30s
```

Full schema: [`specs/CONFIG.md`](../../specs/CONFIG.md).

### Environment Variables

| Variable | Default | Required | Description |
|---|---|---|---|
| `SOCKERLESS_AZF_SUBSCRIPTION_ID` | | **yes** | Azure subscription ID |
| `SOCKERLESS_AZF_RESOURCE_GROUP` | | **yes** | Azure resource group name |
| `SOCKERLESS_AZF_LOCATION` | `eastus` | no | Azure region |
| `SOCKERLESS_AZF_STORAGE_ACCOUNT` | | **yes** | Storage account for function state |
| `SOCKERLESS_AZF_REGISTRY` | | no | ACR registry hostname |
| `SOCKERLESS_AZF_APP_SERVICE_PLAN` | | no | App Service plan name |
| `SOCKERLESS_AZF_TIMEOUT` | `600` | no | Function timeout in seconds |
| `SOCKERLESS_AZF_LOG_ANALYTICS_WORKSPACE` | | no | Log Analytics workspace resource ID |
| `SOCKERLESS_CALLBACK_URL` | | no | Backend URL for reverse agent callbacks |
| `SOCKERLESS_ENDPOINT_URL` | | no | Custom endpoint (for [`simulators/azure`](../../simulators/azure/README.md)) |
| `SOCKERLESS_POLL_INTERVAL` | `2s` | no | Cloud API poll interval |
| `SOCKERLESS_AGENT_TIMEOUT` | `30s` | no | Agent callback timeout |

CLI flags: `-addr` (default `:3375`), `-tls-cert`, `-tls-key`, `-log-level` (default `info`).

## Sample

```bash
$ DOCKER_HOST=tcp://localhost:3375 docker run --rm alpine:3.20 echo "hello from azf"
hello from azf

$ az functionapp show --name sockerless-fn-abc --resource-group sockerless-rg --query state
"Running"

$ az monitor app-insights query \
    --app sockerless-insights --resource-group sockerless-rg \
    --analytics-query 'AppTraces | where AppRoleName == "sockerless-fn-abc"'
[{"Message": "hello from azf", ...}]
```

## Known issues

None open. The shared Azure auth-middleware pattern (`auth.go` wraps the mux to avoid ACR `/v2/` route conflicts) is documented in [`simulators/azure/README.md § Special handling`](../../simulators/azure/README.md).

## What's out of scope

- Premium / Dedicated plan-specific features (durable functions, VNet integration triggers).
- Native runtime modes (Node, Python, Java) — container mode only.
- Event-triggered functions (HTTP only).

## Cloud Notes

- Requires a resource group, storage account, and optionally an App Service plan.
- Authentication uses Azure Default Credentials (`az login`, managed identity, or env vars).
- Uses reverse agent exclusively — Azure Functions cannot accept inbound connections.
- ACR registry must grant the function app `AcrPull` role for private images.
- Timeout max is 600s on Consumption plan, higher on Premium/Dedicated plans.

See also: [`backends/azure-common`](../azure-common/), [`simulators/azure/README.md`](../../simulators/azure/README.md), [`specs/CLOUD_RESOURCE_MAPPING.md`](../../specs/CLOUD_RESOURCE_MAPPING.md).
