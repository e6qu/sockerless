# Sockerless — Roadmap

> **Goal:** Replace Docker Engine with Sockerless for any Docker API client — `docker run`, `docker compose`, TestContainers, CI runners — backed by real cloud infrastructure (AWS, GCP, Azure).

Current state: [STATUS.md](STATUS.md). Bug log: [BUGS.md](BUGS.md). Narrative: [WHAT_WE_DID.md](WHAT_WE_DID.md). Architecture: [specs/](specs/).

## Guiding principles

1. **Docker API fidelity** — match Docker's REST API exactly.
2. **Real execution** — simulators and backends actually run commands; no stubs, fakes, or mocks.
3. **External validation** — proven by unmodified external test suites.
4. **No new frontend abstractions** — Docker REST API is the only interface.
5. **Driver-first handlers** — all handler code through driver interfaces.
6. **LLM-editable files** — source files under 400 lines.
7. **GitHub API fidelity** — bleephub works with unmodified `gh` CLI.
8. **State persistence** — every task ends with state save (PLAN / STATUS / WHAT_WE_DID / BUGS / memory).
9. **No fallbacks, no defers** — every functional gap is a real bug; every bug gets a real fix in the same session it surfaces; cross-cloud sweep on every find.

## Closed phases

- **86** — Simulator parity across AWS + GCP + Azure + Lambda agent-as-handler + Phase C live-AWS ECS validation. See `docs/SIMULATOR_PARITY_{AWS,GCP,AZURE}.md`, [PLAN_ECS_MANUAL_TESTING.md](PLAN_ECS_MANUAL_TESTING.md), and BUGS.md entries 692–722.
- **87** — Cloud Run Jobs → Services path behind `SOCKERLESS_GCR_USE_SERVICE=1` + `SOCKERLESS_GCR_VPC_CONNECTOR`. Closes BUG-715 in code. Live-GCP runbook pending.
- **88** — ACA Jobs → ContainerApps path behind `SOCKERLESS_ACA_USE_APP=1` + `SOCKERLESS_ACA_ENVIRONMENT`. Closes BUG-716 in code. Live-Azure runbook pending.
- **89** — Stateless-backend audit. `specs/CLOUD_RESOURCE_MAPPING.md` for all 7 backends; every cloud-state-dependent callsite uses `resolve*State` helpers; `ListImages` / `ListPods` cloud-derived; Store.Images disk persistence removed. Closes BUG-723/724/725/726.
- **90** — No-fakes/no-fallbacks audit. 11 bugs filed, 8 fixed in-sweep (BUG-729/730/731/732/733/734/735/736/737), 3 scoped as dedicated phases (BUG-744/745/746 → Phase 95/96/97). See WHAT_WE_DID.md for the full table.
- **91** — ECS named volumes + named-volume bind mounts backed by EFS access points on a sockerless-owned filesystem. Simulator `EFSAccessPointHostDir` helper, backend `volumes.go` EFS manager, task defs emit real `EFSVolumeConfiguration`. Completes BUG-735 and the ECS half of BUG-736.

## Pending work

### Live-cloud validation runbooks

- **Phase 87 live-GCP** — parallel to `scripts/phase86/*.sh` for AWS. Needs GCP project + VPC connector. Script the runbook, dispatch via a new workflow, validate `docker run` / `docker exec` / cross-container DNS against Services.
- **Phase 88 live-Azure** — same shape for ACA. Needs Azure subscription + managed environment with VNet integration.
- **Phase 86 Lambda live track** — scripted already, deferred at Phase C closure for session-budget reasons. No architectural blockers.

### Phase 94 — GCF + AZF real volumes (queued)

Sockerless targets only the latest generation of each cloud service (no fallbacks between generations). For GCP Cloud Functions that's v2 (Cloud Run under the hood); for Azure Functions that's Flex Consumption / Premium plan (BYOS Azure Files). Both need real mounts, not NotImplemented — but the cloud APIs each expose a different path than Phase 92 / 93 used, so this is its own phase, not a "copy-paste inherit".

**Sequencing prerequisite** — shared-helper lift (closed 2026-04-21): `gcpcommon.BucketManager`, `azurecommon.FileShareManager`, `awscommon.EFSManager`. CR / ACA / ECS embed them today; GCF / AZF / Lambda embed them when the Phase 94 / 94b backend wiring lands.

#### API-surface realities (read before implementing)

**GCF Functions v2**: the public Functions API (`functionspb.Function.ServiceConfig`) exposes **only `SecretVolumes`** — no first-class GCS (or any other) volume primitive. Every other volume lives on the **underlying Cloud Run Service** that the function *is* under the hood. Google's docs surface this as "advanced per-function configuration via the underlying Cloud Run Service"; it's a sanctioned escape hatch, not a workaround. Two load-bearing consequences:

- **IAM scope grows.** The GCF backend now needs `run.services.get` + `run.services.update` on the same project, in addition to the `cloudfunctions.functions.*` it already has. Operators with narrow GCF-only IAM policies won't be able to mount volumes without a permissions update. Sockerless surfaces this cleanly at `UpdateService` call-time (no retry loops, no silent degradation).
- **Two APIs, one state.** The sockerless-managed invariant (`sockerless-volume-name=<docker-name>` on the bucket) still holds, but the *attachment* lives on the CR Service resource, not the Function resource. If an operator edits the CR Service out-of-band, the function's next revision drops the mount. Phase 94 documents this; Phase 95's reconciliation loop (if we ever add one) would detect drift.

**AZF Flex Consumption**: the file-share attachment API is `sites/<siteName>/config/azurestorageaccounts/<mountName>` — entirely separate from ACA's `managedEnvironmentsStorages`. Schema: `{type: "AzureFiles", accountName, shareName, accessKey, mountPath}`. Load-bearing differences from ACA:

- **Auth model is access-key, not RBAC.** ACA's env-storages authenticate via the managed environment's workload identity against the share's ACL. AZF's azurestorageaccounts embeds the storage account's **plaintext access key** in the site config. Operators must rotate access keys manually; sockerless surfaces the rotation via a `VolumeCreate` cache-invalidation when the key rotates (Phase 94 fetches the key via `StorageAccountsClient.ListKeys` at attach-time, not at config-load, so the fresh key is always used).
- **Mount path is per-site, not per-env.** One `azurestorageaccounts` entry maps one share to one mount path inside one function app. Unlike ACA (where an env-storage is attached to a Job/App volume-mount by name), AZF ties share→path directly on the site resource. `VolumeMount.MountPath` lives inside the `azurestorageaccounts` entry itself.

Both cloud APIs mean sockerless-provisioned volumes are owned by sockerless but **attached on the operator-provisioned compute** (the function app / the CR Service backing the function). Phase 94 doesn't create function apps or CR Services; it hooks into the ones `ContainerCreate` already creates.

#### GCF backend flow

1. `VolumeCreate` — reuse `gcpcommon.BucketManager` to ensure a bucket, labelled identically to CR's.
2. `ContainerCreate` — accept named-volume binds (`volName:/mnt`), reject host-path binds.
3. `ContainerStart`:
   - `Functions.CreateFunction` + `op.Wait` as today.
   - If the request carried any named-volume binds: `Services.GetService(fn.ServiceConfig.Service)` → append `RevisionTemplate.Volumes[]` with `Volume_Gcs` + matching `VolumeMounts` → `Services.UpdateService`.
   - The function's first invocation after UpdateService picks up the mounts (Cloud Run rolls a new revision). If `Services.UpdateService` fails (IAM / quota / invalid bucket), sockerless best-effort-deletes the function so the create appears atomic to the docker client.
4. `VolumeRemove` — reuse `gcpcommon.BucketManager.DeleteForVolume`.

Sim work: the GCP sim's `/v2/projects/.../services/{name}` routes for `GetService` + `UpdateService` already exist (via Phase 87's sim hooks); confirm Volumes round-trip correctly when Phase 94 lands. Cloud Run Functions executor in the sim already translates `Volume{Gcs{Bucket}}` + VolumeMount to host binds (Phase 92).

New GCF client wiring: `GCPClients` gains `Storage *storage.Client` + `Services *run.ServicesClient` (both already present on the Cloud Run backend's `GCPClients`; just add to GCF's).

#### AZF backend flow

1. `VolumeCreate` — reuse `azurecommon.FileShareManager` to ensure a share on the configured storage account.
2. `ContainerCreate` — accept named-volume binds, reject host-path binds.
3. `ContainerStart` — after `WebApps.BeginCreateOrUpdate` creates the function app, fetch the storage key via `StorageAccountsClient.ListKeys` (freshest key at attach-time) and `WebApps.UpdateAzureStorageAccounts` with one `AzureStorageInfoValue{Type=AzureFiles, AccountName, ShareName, AccessKey, MountPath}` entry per bound share.
4. `VolumeRemove` — delete the azurestorageaccounts entry via `WebApps.UpdateAzureStorageAccounts` (omit the mount) then `FileShareManager.DeleteShare`.

Sim work: `simulators/azure/functions.go` grows `sites/<siteName>/config/azurestorageaccounts/<mountName>` CRUD (PUT/GET/DELETE/list) matching the real ARM schema. The sim's AZF executor then looks up each configured mount at invoke-time and translates it to a real host bind using the existing `FileShareHostDir` helper from Phase 93.

New AZF client wiring: `AzureClients` gains `FileShares *armstorage.FileSharesClient` + `StorageAccounts *armstorage.AccountsClient` + `WebApps *armappservice.WebAppsClient.UpdateAzureStorageAccounts` route.

**Older-generation fallthrough**: operators targeting GCF v1 or Azure Functions Consumption plan get a `Config.Validate()` failure at backend boot (`SOCKERLESS_*_REQUIRE_LATEST_GEN` is implicit — sockerless doesn't support the older generations at all). No silent degradation.

**Tests**: SDK + CLI coverage on both sims; integration tests re-enable `TestGCFVolumeOperations` + `TestAZFVolumeOperations` from NotImplemented assertions (same as CR/ACA did) to real lifecycle.

### Phase 94b — Lambda EFS volumes (queued)

Revised from BUG-748: Lambda **does** support EFS mounts via `Function.FileSystemConfigs[]` (each entry pairs an EFS access-point ARN with a local mount path inside the function container). The requirement is Lambda-in-VPC + EFS mount targets in the function's subnets. Not a platform limit — just a more involved setup than ECS's EFS.

Backend flow:
1. `VolumeCreate` — reuse ECS's `volumes.go` EFS manager (the ARM resource is identical; the CRUD lives in `backends/aws-common/volumes.go` after a refactor lift-up from `backends/ecs/volumes.go`).
2. `ContainerCreate` — accept named-volume binds when the Lambda config has at least one subnet configured (VPC + EFS mount targets assumed).
3. `ContainerStart` — add `FileSystemConfigs{Arn: accessPointArn, LocalMountPath: mp}` to `CreateFunctionInput` for each bind. Lambda validates the access point is reachable from the function's subnets at create-time.
4. `VolumeRemove` — same as ECS (delete access point, keep the filesystem).

**Sequencing prerequisite**: refactor `backends/ecs/volumes.go` → `backends/aws-common/volumes.go` first, the same way Phase 94 refactors CR and ACA.

Config-validation: `Config.Validate()` on the Lambda backend rejects `FileSystemConfigs`-carrying requests when `SubnetIds` is empty (matches AWS API behaviour at create-time).

### Phase 95 — FaaS invocation-lifecycle tracker (Lambda + GCF + AZF) (queued)

BUG-744 root cause: FaaS backends' CloudState can't distinguish "function is deployed" from "invocation is running". The *function* is `ACTIVE` regardless of invocation state. `docker wait` / `docker inspect` / `docker ps` therefore can't surface an accurate exited state for short-lived runs, and invocation-failure exit codes are lost (the sim returns HTTP 500 with a body but the backend persists no exit code). Same shape on Lambda, GCF, AZF.

Each backend has a cloud-native signal for per-invocation completion — the fix is to wire that signal through, not to keep a local dictionary. Exactly *one* resource per invocation is authoritative:

| Backend | Invocation resource | Completion signal | Exit-code source |
|---|---|---|---|
| Lambda | `lambda:Invoke` response + CloudWatch Logs `END RequestId` | `InvokeResponse` returns, or log stream gets its terminal `REPORT` line | `FunctionError` (`Unhandled`/`Handled`) ⇒ 1; payload OK ⇒ 0; timeout (`REPORT: … Status: timeout`) ⇒ 124 |
| GCF | HTTP response from `ServiceConfig.Uri` | HTTP status from the invoke POST | 2xx ⇒ 0; 4xx/5xx ⇒ 1 (function-code crash); 408 ⇒ 124 (timeout) |
| AZF | HTTP response from Function App default host | HTTP status from the HTTP trigger POST | Same mapping as GCF |

The invocation-driving goroutine in each backend (`ContainerStart` → goroutine that calls `Invoke` / `POST functionURL`) already knows the outcome — it just drops the exit info today. The right design is:

1. **Capture at the source.** The goroutine records `(containerID, exitCode, stoppedAt)` into a small `InvocationResults sync.Map` on the Server struct when the invocation finishes. This is in-memory, crash-scoped (the invocation was one-shot anyway — post-restart the function's done and the user won't call `docker wait` on it). Explicitly not a revival of `Store.Containers`.
2. **CloudState reads from both.** `GetContainer` / `ListContainers` check `InvocationResults` first — if present, container state is `{Status: "exited", Running: false, ExitCode: N, FinishedAt: T}`. If absent, fall through to cloud lookup (function exists ⇒ `running`; function missing ⇒ `false, nil`).
3. **ContainerStop becomes cooperative.** Write `{ExitCode: 137}` into `InvocationResults` + close the wait channel. Subsequent `Wait` unblocks; `Inspect` shows exited.

What this buys in terms of Docker CLI coverage:
- `docker wait` — returns the real invocation exit code (was always 0).
- `docker inspect` — `State.Status` reflects exited after the invocation completes.
- `docker ps` (no `-a`) — the exited container drops off.
- `docker ps -a` — exited containers appear with their exit code.
- `docker stop` + concurrent `docker wait` — stop unblocks wait (BUG-744 third branch).

Post-restart behaviour (`InvocationResults` gone): `CloudState` sees the cloud function still exists and reports it as `running` until the user removes it. That matches docker's contract for a crashed daemon: state after restart derives from whatever the underlying cloud records — same invariant the rest of the backend already observes.

Tests re-enable: `TestLambdaContainerLifecycle`, `TestLambdaContainerLogsFollowLazyStream`, `TestLambdaContainerStopUnblocksWait`, `TestGCFContainerLifecycle`, `TestGCFArithmeticInvalid`, `TestAZFContainerLifecycle`, `TestAZFArithmeticInvalid` (all deleted as stop-gap for BUG-744).

Simulator work alongside (so live-cloud and simulator behaviour match): the GCP Cloud Functions sim already returns the container's exit code via HTTP status. The AZF sim does the same. The AWS Lambda sim's `Invoke` must set `FunctionError` on non-zero exit and include the last-4KB log tail in the `LogResult` response header — required by the Lambda backend's exit-code derivation.

### Phase 96 — Reverse-agent exec for Cloud Run Jobs + ACA Jobs (queued)

BUG-745 root cause: Cloud Run Jobs and ACA Jobs have no control-plane "attach to running container" API — the Jobs products' entire abstraction is "submit work, read logs after". Lambda has the same gap and solved it by running an agent *inside* the container that dials back over WebSocket (see `agent/cmd/sockerless-lambda-bootstrap`). We port that pattern to Cloud Run + ACA Jobs.

Cloud resource mapping:

| Backend | What the agent needs | Where it lives | How the backend reaches it |
|---|---|---|---|
| `cloudrun` (Jobs) | A pre-baked overlay image containing `sockerless-cloudrun-bootstrap` as its ENTRYPOINT; the bootstrap execs the user's original entrypoint+cmd in a subprocess AND dials `SOCKERLESS_CALLBACK_URL` over WS. | Published to the operator's Artifact Registry; backend references via `SOCKERLESS_GCR_PREBUILT_OVERLAY_IMAGE` env var (mirrors `SOCKERLESS_LAMBDA_PREBUILT_OVERLAY_IMAGE`). | Backend listens on `/v1/cloudrun/reverse` WS endpoint; agent container dials via Serverless VPC Access connector or the existing outbound-egress path. |
| `aca` (Jobs) | Same pattern — `sockerless-aca-bootstrap` image. | Operator's ACR, referenced via `SOCKERLESS_ACA_PREBUILT_OVERLAY_IMAGE`. | Backend listens on `/v1/aca/reverse` WS; agent dials via the Managed Environment's outbound NAT. |

Other scope:
- Three bootstraps share the exec-handling code in `agent/reverse` (already factored out for Lambda); only the startup-time "how do I know my container ID" differs per cloud (Lambda takes `SOCKERLESS_CONTAINER_ID` env, Cloud Run reads `CLOUD_RUN_TASK_ATTEMPT` + job-execution tags, ACA reads `CONTAINER_APP_NAME` + job-execution tags).
- Simulator work: the Cloud Run Jobs + ACA Jobs sims need to honour the prebuilt overlay image the same way the Lambda sim already does — just pull + run locally, no push-to-real-registry hoop.
- Re-enable `TestCloudRunContainerExec` / `TestACAContainerExec` once the overlay images ship.

### Phase 97 — Docker labels on FaaS/GCP backends (queued)

BUG-746 root cause: the Docker label map is serialised as JSON and stored in a single GCP *label* (`sockerless_labels`). GCP label values are `[a-z0-9_-]{0,63}` — JSON's `{`, `:`, `"` are rejected by the real API, the sim silently drops them, so `docker ps --filter label=…` can never match a container whose labels never survived the round-trip. ECS already uses AWS tags (no char restrictions) and works.

Per-cloud resource with arbitrary string values (where Docker labels should actually live):

| Backend | Cloud resource that accepts arbitrary strings | Conventions |
|---|---|---|
| `cloudrun` (Jobs + Services) | **Annotations** on `Job` / `Service` (up to 256 KB per value) | Dedicated keys: `sockerless.dev/labels` for the JSON blob, `sockerless.dev/<kv>` for individual labels large enough to filter on without parsing JSON. |
| `cloudrun-functions` (gcf) | **Annotations** on `Function` | Same convention as `cloudrun`. |
| `aca` (Jobs + Apps) | **Tags** on the resource (Azure tag values allow any string) OR **Metadata** on the container template | Prefer tags — `docker ps --filter` can map to Azure's `resource tags` filter directly. |
| `azure-functions` (azf) | **App Settings** (already arbitrary strings) OR **Tags** | Tags for filterability, App Settings for anything the runtime itself reads. |

Other scope:
- Update `core.TagSet.AsGCPLabels` / `AsGCPAnnotations` split so individual label keys sit in GCP labels when they're `[a-z0-9_-]{0,63}` (docker-ps filter path stays fast), and the JSON blob goes to an annotation only.
- Simulator work: the GCP sim's `Function`, `Job`, `Service` resources all already carry annotation maps; add round-trip coverage for arbitrary string values.
- Re-enable the label-filter assertions in `Test{CloudRun,ACA,GCF,AZF}ArithmeticWithLabels` (removed in BUG-746).

### Phase 98 — Agent-driven container filesystem + introspection ops (queued)

BUG-751 / 752 / 753 root cause: `docker cp` / `docker export` / `docker container stat` / `docker container top` / `docker diff` all need access to a running container's filesystem + process list. Cloud control planes (ECS Describe* / Cloud Run jobs / ACA jobs / Lambda invoke) don't expose any of that; the only path that works is an in-container agent that runs the operation locally and ships the result back over the reverse-agent WebSocket.

Depends on **Phase 96** for CR + ACA (reverse-agent bootstrap). Lambda already has the hook. ECS uses SSM ExecuteCommand as the equivalent channel.

| Docker op | Agent RPC | Cloud resource that carries the result |
|---|---|---|
| `docker cp` / `ContainerGetArchive` | `agent.ArchiveGet(path)` — the agent `tar c <path>` and streams the tarball back | Reverse-agent WS frame `Type=Archive` |
| `docker cp` / `ContainerPutArchive` | `agent.ArchivePut(path, tarball)` — agent untars into container fs | Reverse-agent WS frame `Type=Archive` (direction inverted) |
| `docker container stat` / `ContainerStatPath` | `agent.Stat(path)` — returns `stat` syscall fields | Reverse-agent WS frame `Type=Stat` |
| `docker container top` | `agent.ProcList(psArgs)` — agent runs `ps <psArgs>` and streams output | Reverse-agent WS frame `Type=Top` |
| `docker export` / `ContainerExport` | `agent.ArchiveGet("/")` — entire rootfs | Same as ArchiveGet |
| `docker diff` / `ContainerChanges` | `agent.Changes(imageDigest)` — agent walks image rootfs layers + container rootfs, returns per-path Added/Changed/Deleted entries | Reverse-agent WS frame `Type=Diff` |

Scope:
- `agent/reverse` grows an `Archive` RPC on top of the existing `Exec` plumbing. Per-backend bootstraps (`sockerless-lambda-bootstrap`, `sockerless-cloudrun-bootstrap`, `sockerless-aca-bootstrap`) pick it up automatically because they share the dispatcher.
- ECS uses SSM `StartSession` with the `AWS-RunShellScript` document to run the same commands when no reverse-agent is connected; the existing `backends/ecs/exec_cloud.go` SSM wrapper gains an `archiveGet` helper.
- The `NotImplementedError` returns in each backend's `ContainerGetArchive` / `ContainerPutArchive` / `ContainerStatPath` / `ContainerExport` / `ContainerTop` / `ContainerChanges` get replaced with a call into the reverse-agent helper; when no agent is connected the error stays NotImplemented but mentions the Phase-96 prerequisite.

### Phase 98b — Agent-driven `docker commit` (queued, optional)

BUG-750 — Fargate/Lambda container images are control-plane-owned, but the in-container agent can tarball the rootfs and push it to the operator's registry (ECR / AR / ACR) after Phase 98 lands. Gated behind a config flag (`SOCKERLESS_ENABLE_COMMIT`) because it's a sharp edge: the resulting image isn't what the container started with, so it's not truly reproducible. Users opt in explicitly.

### Phase 100 — Docker backend pod synthesis (queued)

BUG-754 — the Docker backend's `PodCreate/Inspect/List/Exists` return `NotImplementedError` because the local Docker daemon has no pod primitive. Cloud backends (ECS, Cloud Run, ACA) already synthesise pods by tagging containers with a shared `sockerless-pod` label; the Docker backend can do the same against its local `Store.Pods` + Docker's container-label filter.

Scope:
- `backends/docker.PodCreate` — generate pod ID, tag each subsequent container (via `Config.Labels["sockerless-pod"] = name`) when the pod is specified. Store the pod metadata locally.
- `backends/docker.PodList` — list local containers filtered by `label=sockerless-pod=*`, group by pod tag.
- `backends/docker.PodInspect` — look up pod metadata + aggregate container state from Docker's container inspect.
- `backends/docker.PodExists` — filter-by-label existence probe.
- Tests: SDK + CLI (`podman pod create` / `podman pod inspect` / `podman pod ps`) against the Docker backend, matching behaviour with the cloud backends.

### Phase 99 — Agent-driven `docker pause` / `unpause` (queued)

BUG-749 — no cloud control plane exposes per-container pause as a first-class primitive, but the reverse-agent pattern from Phases 95/96 gives sockerless a direct line into the container's process tree. Pause = agent calls `syscall.Kill(userPid, syscall.SIGSTOP)` on the subprocess the bootstrap spawned; Unpause = `SIGCONT`. The agent tracks the PID at user-command fork time, so it knows which process to target.

Scope:
- `agent/reverse` grows `Pause` + `Unpause` RPCs on the existing WebSocket dispatcher. Bootstrap: Lambda today + Phase 96 for CR/ACA.
- ECS Fargate uses SSM Session Manager's `signal` action on the ECS Exec session — same shape, different wire format.
- Backends' `ContainerPause` / `ContainerUnpause` replace the `NotImplementedError` returns with the reverse-agent call.

Dependency chain: Phase 96 must ship before this. ECS already has SSM so its half can land sooner.

### Phase 68 — Multi-Tenant Backend Pools (queued)

Named pools of backends with scheduling and resource limits. `P68-001` done; remaining tasks:

| Task | Description |
|---|---|
| P68-002 | Pool registry (in-memory, each with own BaseServer + Store) |
| P68-003 | Request router (route by label or default pool) |
| P68-004 | Concurrency limiter (per-pool semaphore, 429 on overflow) |
| P68-005 | Pool lifecycle (create/destroy at runtime via management API) |
| P68-006 | Pool metrics (per-pool stats on `/internal/metrics`) |
| P68-007 | Round-robin scheduling (multi-backend pools) |
| P68-008 | Resource limits (max containers, max memory per pool) |
| P68-009 | Unit + integration tests |
| P68-010 | Save final state |

### Phase 78 — UI Polish (queued)

Dark mode, design tokens, error handling UX, container detail modal, auto-refresh, performance audit, accessibility, E2E smoke, documentation.

### Known workarounds to convert to real fixes

- **BUG-721** — sockerless's SSM `acknowledge` format isn't accepted by the live AWS agent, so the backend dedupes retransmitted `output_stream_data` frames by MessageID. Proper fix is to match the agent's ack-validation rules exactly (likely Flags or PayloadDigest semantics); requires live-AWS testing. Pure sim-path isn't affected.

## Future ideas

- GraphQL subscriptions for real-time event streaming.
- Full GitHub App permission scoping.
- Webhook delivery UI.
- Cost controls (per-pool spending limits, auto-shutdown).
