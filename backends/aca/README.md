# Azure Container Apps Backend

Runs Docker containers as Azure Container Apps Jobs and Executions, with Log Analytics for log streaming. Frontend speaks Docker REST API v1.44; backend speaks the Container Apps / Log Analytics / ACR / Managed Identity / Storage APIs.

## Reference adaptors

| Direction | Adaptor | Min version | What it proves |
|---|---|---|---|
| **Frontend (Docker API)** | [Docker Go SDK](https://pkg.go.dev/github.com/docker/docker/client) | v25+ | `docker run` → ACA Job execution via `tcp://localhost:3375`. |
| | [`docker` CLI](https://docs.docker.com/engine/reference/commandline/cli/) | 29.x | Wire-level [Docker REST API v1.44](https://docs.docker.com/engine/api/v1.44/). |
| **Backend (Azure API)** | [`az` CLI](https://learn.microsoft.com/en-us/cli/azure/containerapp/job) | 2.60+ | `az containerapp job execution show`, `az monitor log-analytics query` — operators inspect job state. |
| | [Azure SDK for Go](https://pkg.go.dev/github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appcontainers/armappcontainers) | v3+ | The [Container Apps ARM REST API](https://learn.microsoft.com/en-us/rest/api/containerapps/) calls the backend issues. |
| | [Terraform `azurerm` provider](https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs/resources/container_app_job) | v4+ | `azurerm_container_app_job` provisions the job infra. |

Local development replaces the backend-side upstream with [`simulators/azure`](../../simulators/azure/README.md). The container → ACA Job mapping is documented in [`docs/POD_MATERIALIZATION.md § Azure Container Apps`](../../docs/POD_MATERIALIZATION.md).

## Validation

| Test path | What runs | Last green |
|---|---|---|
| `tests/` (Docker SDK against running backend, ACA profile) | Container lifecycle round-trip via ACA Job. | 2026-05-13 |
| `simulators/azure/sdk-tests/` Container Apps package | ARM calls validated against the sim. | 2026-05-13 |
| `simulators/azure/terraform-tests/` (Docker-only, TLS) | `azurerm_container_app_job` apply / destroy. | 2026-05-13 |
| `make backends/aca/test` | Leaf-Makefile unit + integration suite. | 2026-05-13 |

## Wiring the adaptor

```bash
cd backends/aca && make build
./sockerless-backend-aca --addr :3375 --log-level info &
export DOCKER_HOST=tcp://localhost:3375
```

### Config (config.yaml)

```yaml
environments:
  my-aca:
    backend: aca
    addr: ":3375"
    log_level: info
    azure:
      subscription_id: a1b2c3d4-e5f6-7890-abcd-ef1234567890
      aca:
        resource_group: sockerless-rg
        environment: sockerless
        location: eastus
        log_analytics_workspace: /subscriptions/.../workspaces/sockerless-logs
        storage_account: sockerlessstorage
    common:
      agent_image: sockerless/agent:latest
      agent_token: my-secret-token
      callback_url: https://backend.example.com
      poll_interval: 2s
      agent_timeout: 30s
```

Full schema: [`specs/CONFIG.md`](../../specs/CONFIG.md).

### Environment Variables

| Variable | Default | Required | Description |
|---|---|---|---|
| `SOCKERLESS_ACA_SUBSCRIPTION_ID` | | **yes** | Azure subscription ID |
| `SOCKERLESS_ACA_RESOURCE_GROUP` | | **yes** | Azure resource group name |
| `SOCKERLESS_ACA_ENVIRONMENT` | `sockerless` | no | Container Apps environment name |
| `SOCKERLESS_ACA_LOCATION` | `eastus` | no | Azure region |
| `SOCKERLESS_ACA_LOG_ANALYTICS_WORKSPACE` | | no | Log Analytics workspace resource ID |
| `SOCKERLESS_ACA_STORAGE_ACCOUNT` | | no | Storage account for volumes |
| `SOCKERLESS_ACA_AGENT_IMAGE` | `sockerless/agent:latest` | no | Sidecar agent container image |
| `SOCKERLESS_ACA_AGENT_TOKEN` | | no | Agent authentication token |
| `SOCKERLESS_CALLBACK_URL` | | **yes** | Reverse-agent WebSocket URL the in-App/Job bootstrap dials back to. Empty → backend fails loud at startup (Phase 168 — no management-API exec fallback). |
| `SOCKERLESS_ACA_BOOTSTRAP_TIMEOUT_SEC` | `90` | no | Seconds `ContainerStart` waits for the bootstrap to dial back before failing loud. |
| `SOCKERLESS_ACA_TMPFS_SIZE_MIB` | `2048` | no | Default tmpfs cap (MiB) for `Backing: memory` SharedVolumes. Memory is the default backing on ACA; the per-container memory default raised to `4Gi / 2.0 vCPU` to fit (ACA's CPU:memory 2:1 pairing rule). |
| `SOCKERLESS_ENDPOINT_URL` | | no | Custom endpoint (for [`simulators/azure`](../../simulators/azure/README.md)) |
| `SOCKERLESS_POLL_INTERVAL` | `2s` | no | Cloud API poll interval |
| `SOCKERLESS_AGENT_TIMEOUT` | `30s` | no | Agent health-check timeout |

CLI flags: `-addr` (default `:3375`), `-tls-cert`, `-tls-key`, `-log-level` (default `info`).

## Sample

```bash
$ DOCKER_HOST=tcp://localhost:3375 docker run --rm alpine:3.20 echo "hello from aca"
hello from aca

$ az containerapp job execution list \
    --name sockerless-job --resource-group sockerless-rg --output table
Name              Status    StartTime
sockerless-job-1  Succeeded 2026-05-15T...

$ az monitor log-analytics query \
    --workspace sockerless-logs \
    --analytics-query 'ContainerAppConsoleLogs_CL | where ContainerGroupName_s == "sockerless-job"'
[{"Log_s": "hello from aca", ...}]
```

## Known issues

None open. Azure-specific gotchas (terraform-tests are Docker-only, ACR route conflicts driven by middleware ordering, go-azure-sdk expects 200 on sync creates, storage subdomain routing via dnsmasq) are documented in [`simulators/azure/README.md § Special handling`](../../simulators/azure/README.md).

## What's out of scope

- Container Apps Services (long-running HTTP endpoints) — this backend uses Jobs only.
- ACR Tasks (image builds) — operator-side concern.
- Multi-region deployments.

## Cloud Notes

- Requires a Container Apps environment and resource group to be pre-created.
- Authentication uses Azure Default Credentials (`az login`, managed identity, or env vars).
- Container images must be accessible from ACR or a public registry.
- Supports forward agent (polls execution for IP) and reverse agent (`callback_url`).
- Log Analytics workspace is needed for `docker logs` support.

See also: [`backends/azure-common`](../azure-common/), [`simulators/azure/README.md`](../../simulators/azure/README.md), [`specs/CLOUD_RESOURCE_MAPPING.md § Azure Container Apps`](../../specs/CLOUD_RESOURCE_MAPPING.md).
