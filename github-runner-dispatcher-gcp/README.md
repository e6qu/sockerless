# github-runner-dispatcher-gcp

GCP-native variant of [github-runner-dispatcher](../github-runner-dispatcher/) that creates Cloud Run Jobs directly via `cloud.google.com/go/run/apiv2` instead of shelling out to a docker daemon.

## When to use which

| Use this | When |
|---|---|
| `github-runner-dispatcher` (docker-based) | A sockerless backend (cloudrun / gcf / lambda / ecs) is reachable via `DOCKER_HOST`. The dispatcher hands the runner image to that backend, which translates `docker run` into the underlying cloud primitive. **This is the default for all 4+8 cells today.** |
| `github-runner-dispatcher-gcp` (this) | Operator wants to bypass sockerless and dispatch directly via the GCP control plane — useful when the deployment doesn't run sockerless at all but still wants per-workflow_job ephemeral runners on Cloud Run Jobs. |

Both share the same `--repo`, `--token`, `--config`, `--once`, `--cleanup-only` flag surface and reuse the upstream poller, scopes-check, and registration-token mint via a `replace` directive in `go.mod`.

## Config

`~/.sockerless/dispatcher-gcp/config.toml`:

```toml
[[label]]
name             = "sockerless-cloudrun"
gcp_project      = "my-project"
gcp_region       = "us-central1"
image            = "us-central1-docker.pkg.dev/my-project/runners/runner:latest"
service_account  = "github-runners@my-project.iam.gserviceaccount.com"

[[label]]
name             = "sockerless-gcf"
gcp_project      = "my-project"
gcp_region       = "us-central1"
image            = "us-central1-docker.pkg.dev/my-project/runners/runner-gcf:latest"
service_account  = "github-runners@my-project.iam.gserviceaccount.com"
```

Each entry maps a `runs-on:` label to a (project, region, image, service-account) tuple. Multiple entries with the same project + region share one set of Cloud Run API connections (deduped at runtime).

## State recovery

On startup, the dispatcher calls `Jobs.ListJobs` per (project, region) and rebuilds its seen-set from any Job whose labels match `sockerless-dispatcher-managed-by=github-runner-dispatcher-gcp`. No on-disk state.

## Cleanup

A 2-min ticker (and a `--cleanup-only` mode) deletes Cloud Run Jobs whose `TerminalCondition.State` is `CONDITION_SUCCEEDED`, `CONDITION_FAILED`, or `CONDITION_CANCELLED`. Without this sweep, completed Jobs accumulate indefinitely (default Cloud Run retention is forever).

## Auth

Uses GCP Application Default Credentials (ADC). On a GCE instance / Cloud Run service / Workload Identity-bound k8s pod, ADC resolves automatically. Locally, run `gcloud auth application-default login` first.

## Status

Code-complete (Phase 122 closure). Live-validation pending operator runs.
