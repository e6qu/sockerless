# Sockerless — What We Built

Docker-compatible REST API that runs containers on cloud backends (ECS, Lambda, Cloud Run, GCF, ACA, AZF) or local Docker. 7 backends, 3 cloud simulators, validated against SDKs / CLIs / Terraform.

See [STATUS.md](STATUS.md) for the current phase roll-up, [BUGS.md](BUGS.md) for the bug log, [PLAN.md](PLAN.md) for the roadmap, [specs/](specs/) for architecture specs (start with [specs/SOCKERLESS_SPEC.md](specs/SOCKERLESS_SPEC.md), [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md), [specs/BACKEND_STATE.md](specs/BACKEND_STATE.md)).

## Phase 98 — ContainerTop via reverse-agent (2026-04-23, partial)

First slice of BUG-752: `docker top` now routes through the reverse-agent on every backend that has a bootstrap inside the container.

New shared helpers:
- `agent.ReverseAgentConn.CollectExec(sessionID, cmd, env, workdir) → (stdout, stderr, exit, err)` runs a one-shot command and returns the output accumulated from streamed Message events. Different shape from `BridgeExec` (no caller conn to multiplex) — fits the backend-driven-introspection call pattern.
- `core.RunContainerTopViaAgent(registry, containerID, psArgs) → *api.ContainerTopResponse` + `core.ParseTopOutput` handle the `ps` exec + output parsing.

Per-backend wiring: Lambda, Cloud Run, ACA, GCF, AZF all now return a real `ContainerTopResponse` when a reverse-agent session is registered, or a precise NotImplementedError (`no session registered`) otherwise. GCF + AZF gained the reverse-agent scaffolding (registry, `/v1/<backend>/reverse` WS endpoint, `SOCKERLESS_CALLBACK_URL` + `SOCKERLESS_CONTAINER_ID` env var injection) that Phase 96 had already given CR + ACA.

Remaining Phase 98 methods (`docker cp` / `stat` / `diff` / `export`) follow the same pattern — one-shot `CollectExec` with different argv, wrapped in a backend-agnostic helper in `backends/core`.

## Phase 100 — Docker backend pod synthesis (2026-04-23)

BUG-754 closed. Docker daemon has no native pod primitive but sockerless has tracked cloud containers by the `sockerless-pod` tag since Phase 89. The Docker backend now follows the same convention:
- `PodCreate`/`Inspect`/`Exists` delegate to BaseServer's Store.Pods (in-memory); `PodInspect` falls back to a label-scan reconstruction when Store.Pods doesn't know the pod (post-restart path).
- `PodList` merges in-memory Store.Pods entries with live Docker containers carrying `sockerless-pod`, so a backend restart doesn't drop pods with running containers.
- `PodStart/Stop/Kill/Remove` fan out to the Docker daemon over the SDK using the container IDs stored in Store.Pods (or looked up via the label filter post-restart).

Core `handle_containers.go` injects the `sockerless-pod=<name>` label into the request's Labels BEFORE `ContainerCreate` runs, so every backend (Docker included) tags the underlying resource for cross-restart pod reconstruction.

## Phase 96 — Reverse-agent machinery on backend-core (2026-04-23, partial)

Backend-side scaffolding for the CR + ACA reverse-agent path (BUG-745). Lifted `ReverseAgentRegistry`, `HandleReverseAgentWS`, `ReverseAgentExecDriver`, `ReverseAgentStreamDriver`, `ErrNoReverseAgent` into `backends/core` so Lambda + CR + ACA all share them. Lambda refactored to use the shared types via aliases (behaviour unchanged). CR + ACA servers now own a registry, mount `/v1/cloudrun/reverse` / `/v1/aca/reverse`, wire `Drivers.Exec/Stream` to the shared drivers, and inject `SOCKERLESS_CALLBACK_URL` + `SOCKERLESS_CONTAINER_ID` env vars whenever `Config.CallbackURL` is configured. Without a bootstrap in the container image, Exec/Attach cleanly return exit code 126. Operators can use the existing `sockerless-agent --callback --keep-alive <cmd>` pattern for a first bootstrap overlay.

## Phase 97 — GCP label-value charset compliance (2026-04-21)

BUG-746 closed. Docker labels previously serialised as a single JSON blob into a GCP label value, which GCP rejects because the charset is restricted to `[a-z0-9_-]{0,63}`. `core.AsGCPLabels` now filters values for charset + length and routes failures to `AsGCPAnnotations`. Cloud Run's cloud_state merges `Job.Annotations` / `Service.Annotations` into the ParseLabelsFromTags input. GCF (Functions v2 has no Annotations field on the Function resource) takes a different route — labels are carried as a base64-JSON `SOCKERLESS_LABELS` env var on the function, decoded by cloud_state. `Test{CloudRun,GCF}ArithmeticWithLabels` now assert the round-trip explicitly.

## Phase 94b — Lambda EFS volumes (2026-04-21)

BUG-748 closed. Lambda backend gains the `EFS` client, embeds `volumeState` wrapping `awscommon.EFSManager` (same manager ECS already uses), and accepts `SOCKERLESS_LAMBDA_AGENT_EFS_ID` for operator EFS reuse. `Volume{Create,Inspect,List,Remove,Prune}` now provision sockerless-managed access points. `ContainerCreate` parses `HostConfig.Binds`, rejects host-path binds, and appends one `lambdatypes.FileSystemConfig{Arn, LocalMountPath}` per named volume to the `CreateFunctionInput`. Named volumes require the function to run in a VPC — the backend fails loud if `SOCKERLESS_LAMBDA_SUBNETS` is empty (matches AWS's own validation).

`TestLambdaVolumeOperations` rewritten from the NotImplemented assertion to the real CRUD lifecycle.

## Phase 94 — GCF + AZF real per-cloud volumes (2026-04-21)

`docker volume create` / `docker run -v name:/mnt` now provisions real cloud storage on GCF and AZF, closing the named-volume gap on every FaaS backend:

- **GCF** (Functions v2) gains `Storage *storage.Client` + `Services *run.ServicesClient` in `GCPClients`. `VolumeCreate`/etc. use `gcpcommon.BucketManager` (shared with Cloud Run) to provision one GCS bucket per volume. Because `functionspb.ServiceConfig` only exposes `SecretVolumes`, volumes are attached via the sanctioned escape hatch: after `Functions.CreateFunction` returns, fetch the underlying Cloud Run Service via `fn.ServiceConfig.Service`, append `RevisionTemplate.Volumes[Gcs]` + matching `Container.VolumeMounts`, and `Services.UpdateService`. On failure, the partially-configured function is best-effort-deleted so the create appears atomic.
- **AZF** (Flex Consumption / Premium plan) gains `FileShares` + `StorageAccounts` clients in `AzureClients`. `VolumeCreate`/etc. use `azurecommon.FileShareManager` (shared with ACA) to provision one Azure Files share per volume. After `WebApps.BeginCreateOrUpdate` creates the site, `attachVolumesToFunctionSite` fetches the freshest storage-account access key via `StorageAccounts.ListKeys` (so rotated keys take effect without a restart) and calls `WebApps.UpdateAzureStorageAccounts` with one `AzureStorageInfoValue{accountName, shareName, accessKey, mountPath}` per bound share.
- Host-path bind specs (`/h:/c`) stay rejected on both backends.
- `TestGCFVolumeOperations` + `TestAZFVolumeOperations` rewritten from the NotImplemented assertions to real-lifecycle assertions (create / inspect / list / remove).

## Phase 95 — FaaS invocation-lifecycle tracker (2026-04-21)

BUG-744 closed. New `core.InvocationResult` + `Store.{Put,Get,Delete}InvocationResult` capture each FaaS invocation's exit code + finished-at + error at the source:

- **Lambda** maps `Invoke` result — `FunctionError` → 1, otherwise 0.
- **GCF + AZF** map the HTTP trigger response via `core.HTTPStatusToExitCode` (2xx → 0, 408 → 124, else 1) and `core.HTTPInvokeErrorExitCode` (timeout/deadline → 124, else 1).

Per-backend wiring: `ContainerStart` goroutine writes the result before closing the wait channel; `ContainerStop` / `ContainerKill` write `{ExitCode: 137}` (or `SignalToExitCode` for Kill) so stopped containers surface as exited even though Lambda has no invocation-cancel API; `ContainerRemove` clears the entry; `CloudState.{GetContainer,ListContainers,WaitForExit}` overlay the recorded outcome on `queryFunctions` / `queryFunctionApps` and short-circuit `WaitForExit` with the in-memory result before any cloud polling.

Crash-scoped: restart loses `InvocationResults` and falls back to cloud state (function exists ⇒ `running` until removed). Matches docker's post-daemon-crash contract.

Re-enabled 7 tests that were deleted as a BUG-744 stop-gap: `TestLambdaContainerLifecycle`, `TestLambdaContainerLogsFollowLazyStream`, `TestLambdaContainerStopUnblocksWait`, `TestGCFContainerLifecycle`, `TestGCFArithmeticInvalid`, `TestAZFContainerLifecycle`, `TestAZFArithmeticInvalid`.

## Phase 94 prereq — shared-helper lift (2026-04-21)

Per-cloud volume managers promoted into common modules so FaaS backends can embed them without duplicating provisioning logic:

- `backends/cloudrun/volumes.go` → `backends/gcp-common/volumes.go` as `gcpcommon.BucketManager` (GCS).
- `backends/aca/volumes.go` → `backends/azure-common/volumes.go` as `azurecommon.FileShareManager` (Azure Files). The ACA-specific `ManagedEnvironmentsStorages` linkage stays in aca/volumes.go because AZF will use a different sub-resource (`sites/config/azurestorageaccounts`) for its mount-attach step.
- `backends/ecs/volumes.go` → `backends/aws-common/volumes.go` as `awscommon.EFSManager` (EFS filesystem + access points).

Pure refactor — CR/ACA/ECS behaviour unchanged, unit tests green. A small correctness fix fell out of the ECS lift: `VolumeCreate`'s `fileSystemId` option is now populated even when the operator provides `SOCKERLESS_ECS_AGENT_EFS_ID` (previously it was empty in that branch because the former `efsCachedID` was only set by the `sync.Once` path).

## Phase 92 / 93 — Cloud Run GCS + ACA Azure Files volumes (2026-04-21)

`docker volume create` / `docker run -v name:/mnt` now provisions real cloud storage on both GCP and Azure:

- **Cloud Run (Phase 92)** — one sockerless-owned GCS bucket per volume, labelled `sockerless-managed=true` + `sockerless-volume-name=<docker-name>`. Jobs/Services emit `Volume{Gcs{Bucket}}` in the revision/task template; the sim maps each bucket to a host directory under `$SIM_GCS_DATA_DIR` so tasks bind-mount real files.
- **ACA (Phase 93)** — one Azure Files share per volume inside the operator-configured storage account (`SOCKERLESS_ACA_STORAGE_ACCOUNT`), paired with a `ManagedEnvironmentsStorages` entry so Jobs/Apps can reference it by name. Shares carry `sockerless-managed` + `sockerless-volume-name` metadata. The sim grows `managedEnvironments/<env>/storages/<name>` CRUD + file-share metadata round-trip, and the ACA Jobs executor binds each `(Volume{AzureFile}, VolumeMount)` pair to a real host path under `$SIM_AZURE_FILES_DATA_DIR`.
- Both backends now accept named-volume binds in `ContainerCreate` (previously rejected wholesale — that was the BUG-736 stop-gap). Host-path binds (`-v /h:/c`) stay rejected on both backends because neither Cloud Run nor ACA has a host filesystem to bind from.

## Phase 91 — ECS real volumes via EFS access points (2026-04-21)

`docker volume create` / `-v volname:/mnt` finally provisions real cloud storage on ECS:

- One sockerless-owned EFS filesystem per backend, lazily created on first use (or reused when the operator sets `SOCKERLESS_ECS_AGENT_EFS_ID`), with mount targets in every configured subnet.
- One EFS access point per Docker volume — access-point tags (`sockerless-managed=true` + `sockerless-volume-name=<name>`) carry the mapping so `VolumeInspect` / `VolumeList` / `VolumeRemove` derive from cloud actuals, not an in-memory store.
- Task defs emit `EFSVolumeConfiguration{TransitEncryption=ENABLED, AuthorizationConfig.AccessPointId}` for every named-volume bind; host-path binds (`/h:/c`) stay rejected because Fargate has no host filesystem to bind from.
- Simulator-side `simulators/aws/efs.go` backs every access point with a real host directory under `$SIM_EFS_DATA_DIR` so tasks running locally see persistent files across runs.
- BUG-735 + BUG-736 (ECS half) re-land as this phase.

## Phase 90 — No-fakes / no-fallbacks audit (2026-04-21)

Project-wide audit against the "no fakes, no fallbacks, no placeholders" principle. Every workaround, silent substitution, or placeholder field now gets treated as a bug — not a "known limitation". 11 bugs filed (BUG-729 through BUG-746), 8 fixed in-sweep, 3 scoped as dedicated phases:

| Bug | Area | Resolution |
|---|---|---|
| 729 | SSM ack wire format matches AWS agent (Flags=3 + LSL/MSL layout) | fixed |
| 730 | `ImagePullWithMetadata` no longer synthesises placeholder image records when registry fetch fails | fixed |
| 731 | `VolumeCreate` etc. return `NotImplemented` with a per-cloud message instead of silently storing metadata; Phase 91-94 replace with real provisioning | fixed + superseded on ECS by Phase 91 |
| 732 | Dead `cloudrun.NetworkState.FirewallRuleName` placeholder deleted | fixed |
| 733 | ECS stats no longer fabricates `PIDs: 1` when CloudWatch has no data yet | fixed |
| 734 | ECS `getNamespaceName` propagates the underlying error instead of substituting the raw ID | fixed |
| 735 | ECS rejects host-path bind mounts cleanly; named-volume binds land on EFS (Phase 91) | fixed |
| 736 | Cloud Run + ACA reject bind mounts up-front until Phase 92/93 ship real mount support | fixed (rejection) + queued (provisioning) |
| 737 | `SOCKERLESS_SKIP_IMAGE_CONFIG` opt-out deleted entirely; `ImagePullWithMetadata` requires real metadata | fixed |
| 744 | FaaS CloudState can't signal invocation completion; scope as Phase 95 | scoped |
| 745 | CR/ACA Jobs have no native `docker exec` — scope as Phase 96 (reverse-agent pattern) | scoped |
| 746 | Docker labels don't survive GCP's label-value charset — scope as Phase 97 (GCP annotations / Azure tags) | scoped |

## Phase 89 — Stateless backend audit (2026-04-21)

Per the stateless-backend directive: every cloud backend derives state from cloud actuals; no on-disk state, no canonical in-memory state.

- `specs/CLOUD_RESOURCE_MAPPING.md` formalises how docker concepts (container / pod / image / network / volume) map to each backend's cloud resources + the restart-recovery contract.
- Every cloud-state-dependent callsite across ECS / Lambda / Cloud Run / ACA migrated to `resolve*State` helpers that combine an in-process cache with a cloud-derived fallback.
- `core.CloudImageLister` + `core.CloudPodLister` optional interfaces let `BaseServer.ImageList` / `PodList` merge cloud-derived entries. `ListImages` across all 6 cloud backends (ECR SDK + shared `core.OCIListImages` for AR/ACR). `ListPods` across ECS + cloudrun + aca.
- `resolveNetworkState` reconstructs per-network cloud state (ECS SG + Cloud Map namespace; Cloud Run managed zone; ACA Private DNS + NSG) after a backend restart.
- `Store.Images` disk persistence removed; cache is in-process only.

## Phase 88 — ACA Apps (2026-04-21)

Two execution paths selected by `SOCKERLESS_ACA_USE_APP`:

- **Apps**: `ContainerAppsClient.BeginCreateOrUpdate` with internal ingress + managed environment + min/max = 1. Peers resolve via Private DNS CNAMEs → `ContainerApp.LatestRevisionFqdn`.
- **Jobs** (default): unchanged.

`Config.Validate()` rejects `UseApp=true` without a managed environment.

## Phase 87 — Cloud Run Services (2026-04-21)

Two execution paths selected by `SOCKERLESS_GCR_USE_SERVICE`:

- **Services**: `Services.CreateService` with `INGRESS_TRAFFIC_INTERNAL_ONLY` + VPC connector + scale = 1. Peers resolve via Cloud DNS CNAMEs → `Service.Uri`.
- **Jobs** (default): unchanged.

`Config.Validate()` rejects `UseService=true` without a VPC connector.

## Phase 86 — Simulator parity + Lambda agent-as-handler (2026-04-20, PR #112)

Every cloud-API slice sockerless depends on is a first-class slice in its per-cloud simulator, validated with SDK + CLI + terraform tests (or an explicit `tests-exempt.txt` entry):

- AWS ECR pull-through cache, Lambda Runtime API (per-invocation HTTP sidecar on `host.docker.internal`), Cloud Map backed by real Docker networks.
- GCP Cloud Build + Secret Manager, Cloud DNS private zones backed by real Docker networks.
- Azure Private DNS Zones + NSG + ACR Cache Rules, managed environment backed by real Docker networks.
- Pre-commit testing contract: every `r.Register` addition needs SDK + CLI + terraform coverage.
- Lambda agent-as-handler: `sockerless-lambda-bootstrap` polling loop, overlay-image build in `ContainerCreate`, reverse-agent WebSocket on `/v1/lambda/reverse`.

Phase C live-AWS validated ECS end-to-end in `eu-west-1`: `docker run`, `docker run -d`, FQDN + short-name cross-container DNS, `docker exec`. The live session surfaced 13 real bugs (708–722 minus 715/716); all fixed in-branch. See [PLAN_ECS_MANUAL_TESTING.md](PLAN_ECS_MANUAL_TESTING.md).

## Stack & structure

- **Simulators** — `simulators/{aws,gcp,azure}/`, each a separate Go module. `simulators/<cloud>/shared/` for container + network helpers, `sdk-tests/` for SDK clients, `cli-tests/` for CLI clients, `terraform-tests/` for provider.
- **Backends** — 7 backends (`backends/docker`, `backends/ecs`, `backends/lambda`, `backends/cloudrun`, `backends/cloudrun-functions`, `backends/aca`, `backends/azure-functions`). Each a separate Go module. Cloud-common shared: `backends/{aws,gcp,azure}-common/` (AuthProvider etc.). Core driver + shared types: `backends/core/`.
- **Agent** — `agent/` with sub-commands for the in-container driver + Lambda bootstrap. Shared simulator library: `github.com/sockerless/simulator` (aliased as `sim`).
- **Frontend** — Docker REST API. `cmd/sockerless/` CLI (zero-deps). UI SPA at `ui/` (Bun / React 19 / Vite / React Router 7 / TanStack / Tailwind 4 / Turborepo), embedded via Go `!noui` build tag. 12 UI packages across core + 6 cloud backends + docker backend + docker frontend + admin + bleephub.
- **Tests** — `tests/` for cross-backend e2e, `tests/upstream/` for external test suite replays (act, gitlab-ci-local), `tests/e2e-live-tests/` for runner orchestration, `tests/terraform-integration/`, `smoke-tests/` for per-cloud Docker-backed smokes.
