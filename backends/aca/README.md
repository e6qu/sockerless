# Azure Container Apps Backend

Runs Docker containers as Azure Container Apps Jobs and Executions, with Log Analytics for log streaming.

## Config (config.yaml)

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

## Environment Variables

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
| `SOCKERLESS_CALLBACK_URL` | | no | Backend URL for reverse agent mode |
| `SOCKERLESS_ENDPOINT_URL` | | no | Custom endpoint (for simulators) |
| `SOCKERLESS_POLL_INTERVAL` | `2s` | no | Cloud API poll interval |
| `SOCKERLESS_AGENT_TIMEOUT` | `30s` | no | Agent health-check timeout |

## Quick Start

```sh
go build -o sockerless-backend-aca ./backends/aca/cmd/sockerless-backend-aca
./sockerless-backend-aca -addr :3375 -log-level info
```

Flags: `-addr` (default `:3375`), `-tls-cert`, `-tls-key`, `-log-level` (default `info`).

## Cloud Notes

- Requires a Container Apps environment and resource group to be pre-created.
- Authentication uses Azure Default Credentials (`az login`, managed identity, or env vars).
- Container images must be accessible from ACR or a public registry.
- Supports forward agent (polls execution for IP) and reverse agent (`callback_url`).
- Log Analytics workspace is needed for `docker logs` support.
- See `specs/CONFIG.md` for the full unified config specification.
