# Pod Materialization Per Backend

How a "pod" — the long-lived runner container plus the ephemeral per-step sub-task containers it spawns — actually comes into existence on each of sockerless's 7 backends. Walked through two real-world scenarios:

1. **GitHub Actions runner** (`actions/runner` polls GitHub, then for each `container:` step does `docker create -v /home/runner/_work:/__w … && docker exec …`)
2. **GitLab Runner** (docker executor — `docker create + docker attach` with hijacked stdin per step, `/builds` + `/cache` bind-mounts)

For the architectural matrix ("can this backend serve as a runner host at all?") see [`docs/runner-capability-matrix.md`](runner-capability-matrix.md). For the runner bug catalog see [`docs/RUNNERS.md`](RUNNERS.md). This doc focuses on mechanics — what API calls, what storage primitives, what exec transport.

## The four mechanics each backend must answer

For every backend, "pod materialization" boils down to four questions:

| # | Question | What varies per cloud |
|---|---|---|
| 1 | **Pod primitive** — what is the long-lived container running on? | Fargate task, image-mode Lambda, Cloud Run Service revision, ACA App, AZF Function App, … |
| 2 | **Sub-task spawn** — how does an inner `docker create` materialize? | Same cloud primitive (multi-container Service / App) OR a fresh deploy of the primitive (Fargate task / Lambda function / Function App) |
| 3 | **Workspace data plane** — how do runner + sub-task share `_work` / `/builds`? | EFS access point, GCS bucket (via `gcs-sync`), Azure Files share, host bind-mount |
| 4 | **Exec dispatch** — how do `docker exec` calls reach the sub-task process? | native Docker exec, SSM ExecuteCommand, or reverse-agent WebSocket |

All seven backends answer all four. The summary table at the end shows the answers side-by-side; the per-backend sections below walk the mechanics in detail.

---

## docker

**Pod primitive.** A local Docker container. Sockerless is a thin protocol-translation shim on top of the local daemon.

**Long-lived runner.** Native `docker create` + `docker start` against the host daemon. Entrypoint defaults to whatever the runner image specifies (commonly `tail -f /dev/null` for the runner-as-pod-shell pattern).

**Sub-task spawn.** Native `docker create` on the local daemon. GitHub runner's `docker create -v /home/runner/_work:/__w` becomes a real host bind-mount; GitLab's `--volume /builds:...` + `--volume /cache:...` similarly.

**Workspace data plane.** Host filesystem. The runner container's `_work` (or `/builds`) lives on the host disk; the sub-task sees it via bind-mount. Zero translation.

**Exec dispatch.** Native `docker exec`. Sockerless forwards the API call unchanged. Stdin/stdout/stderr flow over the hijacked HTTP connection.

---

## ecs (AWS Fargate)

**Pod primitive.** ECS Fargate task. One task definition per logical pod.

**Long-lived runner.** Sockerless calls `ecs.RunTask` with a task definition built from the operator's runner image plus a `tail -f /dev/null` entrypoint override. The task is tagged `sockerless-managed=true`, `sockerless-container-id=<id>`, `sockerless-name=<name>` so post-restart recovery can rebuild in-memory state from cloud API queries. CPU/memory come from `SOCKERLESS_ECS_TASK_SIZE` + `SOCKERLESS_ECS_CPU_ARCHITECTURE`.

**Sub-task spawn.** Each `docker create` inside the runner triggers a fresh `ecs.RunTask` on the **same Fargate cluster**, with a task definition that pins the sub-task image. The new task joins the runner via the shared EFS access point — not via ECS task linking. There is no ECS-level "pod" concept; the pod is sockerless's logical bundle, not a cloud-side group.

**Workspace data plane.** EFS access points. Named volumes resolve to access points on a sockerless-managed EFS filesystem (`backends/aws-common/volumes.go::EFSManager`). The operator configures `SOCKERLESS_ECS_SUBNETS` and `SOCKERLESS_ECS_SECURITY_GROUPS` so each task can reach the EFS mount target.

- **GitHub scenario.** `docker create -v /home/runner/_work:/__w` is translated into a `SharedVolumes` entry pointing at the runner's EFS access point root. Sub-paths (`_temp`, `_actions`, `_tool`, `_temp/_github_home`, `_temp/_github_workflow`) collapse onto the same access point (BUG-861 fix).
- **GitLab scenario.** `--volume /builds:...` and `--volume /cache:...` become named-volume references; sockerless creates the access points on demand via `efs.CreateAccessPoint`.

**Exec dispatch.** ECS `ExecuteCommand` over SSM Session Manager (`backends/ecs/exec_cloud.go`). Sockerless waits for `ManagedAgents[ExecuteCommandAgent].LastStatus == RUNNING` before invoking — Fargate's SSM agent starts asynchronously and immediate exec returns "agent not found" (BUG-853 fix). The response is mux-framed (Docker's stdcopy format) and returned to the caller.

GitLab's hijacked-stdin pattern is handled with the `stdinPipes` map: `ContainerStart` reads the buffered stdin bytes (written by `docker attach`) and bakes them into the task definition's `Cmd` override before calling `ecs.RunTask` (BUG-859 fix). Fargate has no remote-stdin channel for a running task, so stdin must be embedded at task launch.

---

## lambda (AWS)

**Pod primitive.** A Lambda function in **image mode** (not the managed runtime). The user's image is wrapped with a `sockerless-lambda-bootstrap` overlay that captures the original entrypoint/cmd as env vars and exec's them inside the Lambda Runtime API loop.

**Long-lived runner.** `lambda.CreateFunction` deploys the runner-Lambda. The overlay-injected image is built once per content hash via CodeBuild (operator configures `SOCKERLESS_CODEBUILD_PROJECT` + `SOCKERLESS_BUILD_BUCKET`) and cached in ECR. The function carries `FileSystemConfig` pointing at a shared EFS access point so the runner + sub-task Lambdas all see the same workspace (BUG-862 fix: runner-Lambda bakes `sockerless-backend-lambda`, NOT `sockerless-backend-ecs` — sub-task dispatch stays on the same primitive).

The Lambda **15-minute invocation limit** is the dominant constraint. The runner's `tail -f /dev/null` keep-alive cannot last longer than 15 min; longer jobs must use ECS. See `docs/RUNNERS.md` § Architecture for the trade-off.

**Sub-task spawn.** Each `docker create` for a sub-task calls `lambda.CreateFunction` to create a **new** Lambda function (not re-invoke the runner). Sub-tasks are image-mode containers like the runner, with their own bootstrap. The pattern is fresh-function-per-sub-task rather than fresh-invocation-of-existing-function, which keeps per-sub-task isolation.

**Workspace data plane.** EFS access points (same machinery as ECS, shared via `aws-common/volumes.go`). The runner-Lambda's `/home/runner/_work` is symlinked to `/mnt/runner-workspace` (BUG-862 fix — Lambda's filesystem is read-only outside `/tmp`, so the workspace gets relocated). `SOCKERLESS_LAMBDA_SHARED_VOLUMES` carries multiple `name=path=<apid>` entries pointing at the **same** access-point root so sockerless's per-volume access-point lookup short-circuits onto the shared mount (BUG-861 fix — Lambda allows only a single `FileSystemConfig` per function).

**Exec dispatch.** Reverse-agent WebSocket only. The bootstrap dials `SOCKERLESS_CALLBACK_URL`, registers with `SOCKERLESS_CONTAINER_ID`, and sockerless tunnels exec requests over that WebSocket. The old Invoke-envelope Path B was removed in Phase 168 because it silently converted each `docker exec` step into a fresh cloud invocation when the agent was missing.

GitLab's hijacked-stdin pattern must ride the same registered reverse-agent/session path. If the bootstrap does not register before `SOCKERLESS_LAMBDA_BOOTSTRAP_TIMEOUT_SEC`, `ContainerStart` fails loudly.

---

## cloudrun (GCP Cloud Run Services)

**Pod primitive.** A **multi-container Cloud Run Service revision** when `SOCKERLESS_GCR_USE_SERVICE=1` (the runner default). Cloud Run Services support sidecars natively — runner + service containers (postgres, redis, etc.) all sit in one revision sharing `localhost`. Cloud Run Jobs are also available but execution-scoped, so they don't fit the long-lived runner pattern.

**Long-lived runner.** Sockerless builds a revision template that bundles the runner container plus any deferred user-defined-network members as sidecars. `run.Services.CreateService` deploys the Service. Each container in the revision is wrapped with `sockerless-cloudrun-bootstrap` so the bootstrap (an HTTP server on `:8080`) becomes the exec entrypoint; the user entrypoint runs as a subprocess of the bootstrap.

Service revisions scale to zero between execs (`MinInstanceCount: 0, MaxInstanceCount: 1`) so the pod doesn't pin regional quota between job steps.

**Sub-task spawn.** Same Service path — a new `docker create` for a sub-task script-runner triggers a **new revision** of the **same Service** (or a new Service entirely when the network membership differs). `shouldDeferOrMaterializeNetworkPod()` decides: service containers (no `OpenStdin`) are deferred until a script-runner (`OpenStdin=true`) arrives; when it does, all deferred members are bundled into one revision.

**Workspace data plane.** GCS buckets via the **`gcs-sync` storage backing driver** (`backends/gcp-common/storage_gcssync.go`). Phase 92 deregistered `gcs-fuse` on Cloud Run because Cloud Run rejects the cache-TTL gcsfuse mount flags needed for cross-task safety (BUG-944 → BUG-987). The gcs-sync flow:

1. **PreExec** — runner-task tars its local volume mount, uploads to a GCS object keyed by exec ID, returns an env hint `SOCKERLESS_WORKSPACE_OBJECT=gs://bucket/exec-<id>.tar.gz`.
2. **Bootstrap receives the exec request** over the reverse-agent path, sees the hint, downloads + untars to the local mount before running the subprocess.
3. **Subprocess runs**, mutates the workspace.
4. **Bootstrap tars + uploads** the post-run workspace back to the same object.
5. **PostExec** — runner-task downloads + untars to pick up mutations, deletes the object.

This is per-step granularity. Cross-step persistence rests on GCS object durability, not on a live shared filesystem.

**Exec dispatch.** Reverse-agent WebSocket only. The Cloud Run Service URL is still invoked once to start the overlay bootstrap and keep the instance warm, but per-step `docker exec` goes through the registered WebSocket session. Simulator validation added in Phase 168.9 proves a stock image is overlay-wrapped, `ContainerStart` waits for `/v1/cloudrun/reverse`, and `docker exec` returns over Docker's stdcopy-framed response.

GitLab's hijacked-stdin lands on the same reverse-agent stream; there is no Service-URL exec fallback.

---

## cloudrun-functions (GCF / Cloud Functions Gen2)

**Pod primitive.** A Cloud Function (Gen2) — which is itself a Cloud Run Service under the hood. Same multi-container Service capability via an "escape hatch": after `functions.CreateFunction`, sockerless calls `run.Services.UpdateService` on the function's underlying Service to swap in the real overlay image and attach sidecars / volumes (Cloud Functions' `ServiceConfig` exposes only SecretVolumes; sidecars + GCS Volumes must be added at the Cloud Run layer).

**Long-lived runner.** `functions.CreateFunction` deploys the cloud-required bootstrap source for the selected Buildpacks runtime. Cloud Build compiles it to an initial image so the Cloud Functions API can create the function. Sockerless then `Run.Services.UpdateService`'s the function's underlying Service to replace that initial image with the user's image + bootstrap overlay. The overlay is cached in Artifact Registry keyed by `sha256(user-image, bootstrap-binary, user-cmd, user-entrypoint, user-workdir)` so subsequent functions with the same shape reuse the existing image.

**Function pool reuse.** Functions with the same overlay hash are retained after `docker rm` and reused for subsequent containers — amortized startup. Allocation is tracked via cloud-side labels (`sockerless_allocation=<containerID>`); sockerless is stateless and reads the labels to determine pool membership.

**Sub-task spawn.** Same deploy sequence per `docker create`: pool query → cache check → build if needed → CreateFunction → UpdateService. Multi-container pods: `materializePodService()` (`backends/cloudrun-functions/pod_service.go`) builds overlay images for all pod members in parallel and deploys as one multi-container Cloud Run Service revision.

**Workspace data plane.** GCS buckets via `gcs-sync` (same as Cloud Run). Volumes attach via `attachVolumesToFunctionService` (`backends/cloudrun-functions/volumes.go`) — the function itself has no native Volumes primitive, but the underlying Service does. Idempotent attach: compare existing `volumes[].name + bucket + mountOptions` vs requested; replace stale entries.

**Exec dispatch.** Reverse-agent WebSocket only. The Function/underlying Service URL is invoked to start the overlay bootstrap, and the bootstrap registers back to `/v1/gcf/reverse` before `ContainerStart` returns. Per-step `docker exec` then runs through the WebSocket session. Simulator validation added in Phase 168.9 covers `ContainerStart` registration plus `docker exec` exit-code inspection.

---

## aca (Azure Container Apps)

**Pod primitive.** An ACA **App** revision when `SOCKERLESS_ACA_USE_APP=1` (the runner default). App revisions support multi-container natively — same shape as Cloud Run Services. ACA Jobs are also available but execution-scoped.

**Long-lived runner.** `armappcontainers.ContainerAppsClient.CreateOrUpdate` deploys an App with the runner container + any deferred network members as sidecars. Each container is wrapped with `sockerless-aca-bootstrap`. The App is tagged `sockerless-managed=true`, `sockerless-container-id=<id>` for recovery.

**Sub-task spawn.** Each `docker create` calls `CreateOrUpdate` to deploy a new App revision (or a new App entirely, depending on network membership). The deferred-network-pod pattern mirrors Cloud Run / GCF.

**Workspace data plane.** Azure Files shares (`backends/azure-common/volumes.go`). Named volumes map to Azure Files shares in a sockerless-managed storage account (`SOCKERLESS_ACA_STORAGE_ACCOUNT`). `ShareVolumeConfiguration` injects into the App template. Persistent across revisions — the share is remounted, not recreated, so workspace mutations survive between job steps.

**Exec dispatch.** ACA Apps, which are the runner path when `SOCKERLESS_ACA_USE_APP=1`, use the reverse-agent WebSocket registered by the bootstrap. ACA Job execution is a separate execution-scoped primitive and is not the runner fallback path.

GitLab's hijacked-stdin lands in the reverse-agent multiplexed channel for Apps.

---

## azure-functions (AZF / Azure Functions)

**Pod primitive.** A Function App (Linux container plan) running a single image-mode container. Each Function App is an HTTP-triggered workload; the bootstrap listens on `0.0.0.0:8080` and the Function App host triggers it on every request.

**Long-lived runner.** `armappservice.WebAppsClient.CreateOrUpdate` deploys a Function App with the user's image + `sockerless-azf-bootstrap` overlay. The Function App is tagged `sockerless-managed=true`, `sockerless-container-id=<id>` for recovery.

**Sub-task spawn — supervisor-in-overlay pattern.** AZF is the most constrained backend: Function Apps run a single container, no native sidecar primitive, and don't expose `CAP_SYS_ADMIN`. Multi-container pods materialize via a **supervisor-in-overlay**: all pod members are baked into one image via an overlay Dockerfile that merges their rootfs layers under distinct directories, and a supervisor bootstrap launches each as a subprocess. Pod containers share net/IPC/UTS namespaces but not mount/PID (no namespacing privileges).

This is a real limitation, not a workaround — it's the cloud-native answer for AZF given the platform constraints. See `specs/CLOUD_RESOURCE_MAPPING.md` § Azure Functions for the rationale.

**Workspace data plane.** Azure Files shares (same `azure-common` machinery as ACA). `SOCKERLESS_AZF_STORAGE_ACCOUNT` configures the storage account. Persistent across invocations.

**Exec dispatch.** Reverse-agent over WebSocket. The bootstrap registers a reverse-agent WS during startup; sockerless tunnels exec requests over it. There is no alternate envelope-POST exec path.

---

## Scenario walk-throughs

### GitHub Actions runner — per-backend

The runner pattern: `actions/runner` polls GitHub, then for each `container:` step does `docker create -v /home/runner/_work:/__w <step-image>` and `docker exec <container> sh /_temp/step-<id>.sh`. Files written to `/home/runner/_work` by step N must be visible to step N+1.

| Step | docker | ecs | lambda | cloudrun | gcf | aca | azf |
|---|---|---|---|---|---|---|---|
| Runner container created | `docker create` | `ecs.RunTask` with `tail -f /dev/null` | `lambda.CreateFunction` (image mode, overlay) | `run.Services.CreateService` (multi-container revision) | `functions.CreateFunction` + `UpdateService` swap | `ContainerAppsClient.CreateOrUpdate` | `WebAppsClient.CreateOrUpdate` |
| Step container created | `docker create` | `ecs.RunTask` (sub-task) | `lambda.CreateFunction` (sub-task) | New revision of same Service | New function + `UpdateService` (pool-reusable) | New App revision | New Function App (or supervisor-in-overlay) |
| `_work` mount | host bind | EFS access point | EFS access point | GCS bucket via `gcs-sync` (per-exec tar) | GCS bucket via `gcs-sync` | Azure Files share | Azure Files share |
| `docker exec` for step | native | SSM ExecuteCommand (waits for agent) | Reverse-agent WS | Reverse-agent WS | Reverse-agent WS | Reverse-agent WS | Reverse-agent WS |
| Cross-step persistence | Native (same host) | EFS mount stays attached | EFS persists across invokes | GCS object tar/untar per step | GCS object tar/untar per step | Azure Files share persists | Azure Files share persists |

### GitLab Runner — per-backend

The runner pattern: `gitlab-runner` polls GitLab, then per stage does `docker create --volume /builds:... --volume /cache:...`, `docker attach <container>` (hijacks stdin), writes the stage script to stdin, reads stdout until EOF, then `docker stop`. The same container ID may be re-`start`ed for the next stage (BUG-858).

| Step | docker | ecs | lambda | cloudrun | gcf | aca | azf |
|---|---|---|---|---|---|---|---|
| Build container created | `docker create` | `ecs.RunTask` | `lambda.CreateFunction` | `CreateService` (build + helper + service sidecars) | `CreateFunction` + `UpdateService` (build + helper + sidecars) | `CreateOrUpdate` (multi-container App) | `CreateOrUpdate` (supervisor overlay) |
| Stdin script delivery | native attach | `stdinPipes` map → bake into task def `Cmd` (BUG-859) | Reverse-agent stream | Reverse-agent stream | Reverse-agent stream | WebSocket stream | WebSocket stream |
| `/builds` mount | host bind | EFS access point | EFS access point | GCS bucket via `gcs-sync` | GCS bucket via `gcs-sync` | Azure Files share | Azure Files share |
| `/cache` mount | host bind | EFS access point | EFS access point | GCS bucket via `gcs-sync` | GCS bucket via `gcs-sync` | Azure Files share | Azure Files share |
| Per-stage re-`start` | native | `ContainerStart` re-resolves task ARN, opens new stdin pipe | Re-invoke same function with new stdin pipe | New revision of same Service | New invoke of same function | New App revision | Re-invoke same Function App |

---

## Summary table

| Dimension | docker | ecs | lambda | cloudrun | gcf | aca | azf |
|---|---|---|---|---|---|---|---|
| **Pod primitive** | Local container | Fargate task | Lambda function (image mode) | Multi-container Service revision | Cloud Function Gen2 (underlying Service) | App revision | Function App (single container + supervisor overlay) |
| **Long-lived shell** | `tail -f /dev/null` | task stays running | bootstrap + Runtime API loop | revision stays warm | function stays warm (pool) | revision stays active | Function App HTTP handler |
| **Sub-task primitive** | new `docker create` | new `ecs.RunTask` | new `lambda.CreateFunction` | new revision of same Service | new Function (pool-reusable) + `UpdateService` | new App revision | new Function App (or supervisor overlay) |
| **Workspace storage** | host bind | EFS access point | EFS access point | GCS bucket via `gcs-sync` | GCS bucket via `gcs-sync` | Azure Files share | Azure Files share |
| **Exec dispatch** | native `docker exec` | SSM ExecuteCommand | reverse-agent WS | reverse-agent WS | reverse-agent WS | reverse-agent WS | reverse-agent WS |
| **Cross-step persistence** | host fs | EFS mount stays | EFS mount stays | GCS object tar/untar | GCS object tar/untar | Azure Files share | Azure Files share |
| **Hard limit** | (none) | (none — Fargate is long-lived) | 15-min invocation cap | (none — Services are long-lived) | (none — same as cloudrun) | (none — Apps are long-lived) | (none — but supervisor-overlay limits CAP_SYS_ADMIN-dependent workloads) |

---

## Code paths

| Concern | File |
|---|---|
| Pod provisioning + activation | `backends/<cloud>/backend_impl.go::ContainerCreate` / `ContainerStart` |
| Multi-container materialization (cloudrun/gcf) | `backends/cloudrun/network_pod.go::shouldDeferOrMaterializeNetworkPod`; `backends/cloudrun-functions/pod_service.go::materializePodService` |
| Storage drivers | `backends/aws-common/volumes.go::EFSManager`; `backends/gcp-common/storage_gcssync.go`; `backends/azure-common/volumes.go::AzureFilesEphemeralDriver` |
| Exec dispatch | `backends/ecs/exec_cloud.go`; `backends/core/reverse_agent.go`; `backends/lambda/exec_driver.go`; `backends/<faas>/server.go` reverse-agent driver wiring; `backends/<faas>/backend_impl*.go` / `backend_delegates.go` `ExecStart` paths |
| Stdin pipe machinery | `backends/<cloud>/stdin_pipes.go` (ECS, Lambda) |
| Bootstrap binaries | `agent/cmd/sockerless-{cloudrun,gcf,lambda,aca,azf}-bootstrap/` |
| Authoritative cloud-side mapping | `specs/CLOUD_RESOURCE_MAPPING.md` |
