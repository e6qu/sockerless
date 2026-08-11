# github-runner-dispatcher-azure

Azure-native variant of [github-runner-dispatcher](../github-runner-dispatcher-aws/) that creates Azure Container Apps Jobs directly via `armappcontainers` instead of shelling out to a docker daemon.

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

# Optional knobs (per label):
# runner_job_timeout = 3600   # ACA ReplicaTimeout on the runner-task, seconds (default 3600)
# max_concurrent     = 10     # cap on live runner Jobs in this (subscription, resource group); 0 = unbounded
```

`environment` is the full ARM ID of the pre-provisioned Container Apps Environment that hosts the Jobs. `managed_identity` is the user-assigned managed identity the Job execution runs as (required for the Job to pull from a private ACR or write to other ARM resources).

Runner-task resources are 2 vCPU / 4 Gi (the ACA default 0.5/1Gi is too small for a runner that compiles real workloads).

## Deployable shape

For running the dispatcher itself as an ACA App: `--repo` can come from `$REPO`, the PAT from `$GITHUB_TOKEN` (bind via ACA secret), and setting `$PORT` starts a `/healthz` responder for container probes. Scope verification retries with backoff (honoring GitHub rate-limit hints with the +10% +1s buffer) instead of exiting on a transient 403/429, and the poll loop sleeps out rate-limit windows.

## State recovery

On startup, the dispatcher calls `Jobs.NewListByResourceGroupPager` per (subscription, resource group) and rebuilds its seen-set from any Job whose tags include `sockerless-dispatcher-managed-by=github-runner-dispatcher-azure`. No on-disk state.

## Cleanup

A 2-min ticker (and a `--cleanup-only` mode):

- deletes ACA Jobs whose latest **execution** is terminal (JobExecution `Status` of `Succeeded`, or `Failed`/`Stopped`/`Degraded`). Keyed off the execution, never `Properties.ProvisioningState` — that field tracks the ARM resource's provisioning and reads `Succeeded` right after create, while the execution is still running (deleting the Job then would kill the in-flight CI job);
- deletes Jobs that never got an execution (`BeginStart` failed) once they're older than a 15-min grace;
- deregisters offline `dispatcher-*` runners on the GitHub side (ephemeral runners that died without completing leave zombie registrations).

Without the Job sweep, the resource group accumulates one Job resource per workflow_job (the Job itself is preserved between executions in ACA's resource model).

## Registration-token handling

The per-spawn GitHub registration token rides as an ACA Job **secret** with a `secretRef` env binding — it never appears in the Job's plain env (which any ARM reader on the resource can see; secret values are write-only on GET and reading them back needs the separately-RBAC'd `listSecrets` action).

## Auth

Uses Azure Default Credential chain (`azidentity.NewDefaultAzureCredential`). On a managed-identity-bound Azure VM / Container App / AKS pod, the chain resolves automatically. Locally, run `az login` first.

## Status

Code-complete (Phase 122b closure). Live-validation pending operator runs.
