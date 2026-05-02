# github-runner-dispatcher-azure

Azure-native variant of [github-runner-dispatcher](../github-runner-dispatcher/) that creates Azure Container Apps Jobs directly via `armappcontainers` instead of shelling out to a docker daemon.

## When to use which

| Use this | When |
|---|---|
| `github-runner-dispatcher` (docker-based) | A sockerless backend (aca / azure-functions / cloudrun / gcf / lambda / ecs) is reachable via `DOCKER_HOST`. The dispatcher hands the runner image to that backend, which translates `docker run` into the underlying cloud primitive. |
| `github-runner-dispatcher-azure` (this) | Operator wants to bypass sockerless and dispatch directly via the Azure ARM control plane — useful when the deployment doesn't run sockerless at all but still wants per-workflow_job ephemeral runners on ACA Jobs. |

Same flag surface (`--repo`, `--token`, `--config`, `--once`, `--cleanup-only`) and reuses the upstream poller, scopes-check, and registration-token mint via a `replace` directive in `go.mod`.

## Config

`~/.sockerless/dispatcher-azure/config.toml`:

```toml
[[label]]
name             = "sockerless-aca"
subscription_id  = "00000000-0000-0000-0000-000000000000"
resource_group   = "sockerless-runners-rg"
environment      = "/subscriptions/.../managedEnvironments/sockerless-runners-env"
location         = "eastus2"
image            = "myacr.azurecr.io/runners/runner:latest"
managed_identity = "/subscriptions/.../userAssignedIdentities/runner-id"
```

`environment` is the full ARM ID of the pre-provisioned Container Apps Environment that hosts the Jobs. `managed_identity` is the user-assigned managed identity the Job execution runs as (required for the Job to pull from a private ACR or write to other ARM resources).

## State recovery

On startup, the dispatcher calls `Jobs.NewListByResourceGroupPager` per (subscription, resource group) and rebuilds its seen-set from any Job whose tags include `sockerless-dispatcher-managed-by=github-runner-dispatcher-azure`. No on-disk state.

## Cleanup

A 2-min ticker (and a `--cleanup-only` mode) deletes ACA Jobs whose `Properties.ProvisioningState` is `Succeeded`, `Failed`, or `Canceled`. Without this sweep, the resource group accumulates one Job resource per workflow_job (the Job itself is preserved between executions in ACA's resource model).

## Auth

Uses Azure Default Credential chain (`azidentity.NewDefaultAzureCredential`). On a managed-identity-bound Azure VM / Container App / AKS pod, the chain resolves automatically. Locally, run `az login` first.

## Status

Code-complete (Phase 122b closure). Live-validation pending operator runs.
