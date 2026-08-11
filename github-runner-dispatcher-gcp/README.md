# github-runner-dispatcher-gcp

GCP-native variant of [github-runner-dispatcher-aws](../github-runner-dispatcher-aws/) that creates Cloud Run Jobs directly via `cloud.google.com/go/run/apiv2` instead of shelling out to a docker daemon.

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

# Optional knobs (per label):
# runner_job_timeout = 3600   # Cloud Run task timeout on the runner-task, seconds (default 3600)
# max_concurrent     = 10     # cap on live runner Jobs in this (project, region); 0 = unbounded
```

Each entry maps a `runs-on:` label to a (project, region, image, service-account) tuple. Multiple entries with the same project + region share one set of Cloud Run API connections (deduped at runtime).

## State recovery

On startup, the dispatcher calls `Jobs.ListJobs` per (project, region) and rebuilds its seen-set from any Job whose labels match `sockerless-dispatcher-managed-by=github-runner-dispatcher-gcp`. No on-disk state.

## Cleanup

A 2-min ticker (and a `--cleanup-only` mode):

- deletes Cloud Run Jobs whose latest **execution** is terminal (`EXECUTION_SUCCEEDED` / `EXECUTION_FAILED`). Keyed off the execution, never `TerminalCondition.State` — that field tracks the Job *definition's* reconciliation and reads ready right after create, while the execution is still running;
- deletes Jobs that never got an execution (`RunJob` failed) once they're older than a 15-min grace;
- sweeps orphan `sockerless-svc-*` pod-Services whose owning runner-task is gone or terminal;
- deregisters offline `dispatcher-*` runners on the GitHub side (ephemeral runners that died without completing leave zombie registrations).

## Registration-token handling

The per-spawn GitHub registration token is stored as a Secret Manager secret (`<job-id>-regtoken`, 2-h TTL) and bound to the Job's `RUNNER_REG_TOKEN` env via a secret reference — it never appears in the Job resource's plain env (which any `run.jobs.get` reader can see). The secret is deleted with its Job; the TTL is the crash-path backstop.

## Auth

Uses GCP Application Default Credentials (ADC). On a GCE instance / Cloud Run service / Workload Identity-bound k8s pod, ADC resolves automatically. Locally, run `gcloud auth application-default login` first.

IAM the two identities need:

| Identity | Roles |
|---|---|
| Dispatcher (ADC) | `roles/run.admin` (create/run/delete Jobs, delete Services), `roles/secretmanager.admin` scoped to the project (create/delete `*-regtoken` secrets), `roles/iam.serviceAccountUser` on the runner service account |
| Runner service account (`service_account`) | `roles/secretmanager.secretAccessor` (read the token secret at execution start) plus the backend roles listed in `specs/CLOUD_RESOURCE_MAPPING.md` |

## Status

Code-complete (Phase 122 closure). Live-validation pending operator runs.
