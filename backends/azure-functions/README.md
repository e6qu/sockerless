# Azure Functions Backend

Runs Docker containers as Azure Function Apps with custom container images, with Log Analytics for log streaming.

## Config (config.yaml)

```yaml
environments:
  my-azf:
    backend: azf
    addr: ":9100"
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

## Environment Variables

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
| `SOCKERLESS_ENDPOINT_URL` | | no | Custom endpoint (for simulators) |
| `SOCKERLESS_POLL_INTERVAL` | `2s` | no | Cloud API poll interval |
| `SOCKERLESS_AGENT_TIMEOUT` | `30s` | no | Agent callback timeout |

## Quick Start

```sh
go build -o sockerless-backend-azf ./backends/azure-functions/cmd/sockerless-backend-azf
./sockerless-backend-azf -addr :9100 -log-level info
```

Flags: `-addr` (default `:9100`), `-tls-cert`, `-tls-key`, `-log-level` (default `info`).

## Cloud Notes

- Requires a resource group, storage account, and optionally an App Service plan.
- Authentication uses Azure Default Credentials (`az login`, managed identity, or env vars).
- Uses reverse agent exclusively -- Azure Functions cannot accept inbound connections.
- ACR registry must grant the function app `AcrPull` role for private images.
- Timeout max is 600s on Consumption plan, higher on Premium/Dedicated plans.
- See `specs/CONFIG.md` for the full unified config specification.
