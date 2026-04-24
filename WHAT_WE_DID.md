# Sockerless — What We Built

Docker-compatible REST API that runs containers on cloud backends (ECS, Lambda, Cloud Run, GCF, ACA, AZF) or local Docker. 7 backends, 3 cloud simulators, validated against SDKs / CLIs / Terraform.

See [STATUS.md](STATUS.md) for the current phase roll-up, [BUGS.md](BUGS.md) for the bug log, [PLAN.md](PLAN.md) for the roadmap, [specs/](specs/) for architecture (start with [specs/SOCKERLESS_SPEC.md](specs/SOCKERLESS_SPEC.md), [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md), [specs/BACKEND_STATE.md](specs/BACKEND_STATE.md)).

## Phases 96 / 98 / 98b / 99 / 100 / 101 / 102 + 13-bug audit sweep (2026-04-24, PR #115)

Shipped on the merged `phase96-onward` branch. 770 total bugs after this PR — 769 fixed, 0 open, 1 false positive (BUG-747 audit umbrella).

**Phase 96 — reverse-agent exec for CR/ACA Jobs.** `backends/core` owns the shared `ReverseAgentRegistry`, `HandleReverseAgentWS`, `ReverseAgent{Exec,Stream}Driver`, `ErrNoReverseAgent`. CR/ACA/GCF/AZF mount `/v1/<backend>/reverse`, wire `Drivers.Exec`/`Drivers.Stream`, inject `SOCKERLESS_CALLBACK_URL`. Closes BUG-745.

**Phase 98 — agent-driven filesystem + introspection ops.** `core.RunContainer{Top,StatPath,GetArchive,PutArchive,Export,Changes}ViaAgent` wrap `ps`, `stat`, `tar`, `find` over the reverse-agent; shared parsers in `container_{top,statpath,changes}.go`. Wired across every FaaS backend. Closes BUG-751/752/753.

**Phase 98b — agent-driven `docker commit` (opt-in).** `core.CommitContainerViaAgent` runs `find / -xdev -newer /proc/1 -printf '%p\0'` + `tar -cf - --null -T -` through the reverse-agent to capture a proper diff layer stacked on the source image's rootfs. Gated behind `SOCKERLESS_ENABLE_COMMIT=1`. Deletions not captured (find-based — documented limitation, not a silent fallback). Closes BUG-750.

**Phase 99 — agent-driven pause/unpause.** Bootstraps publish the user-process PID to `/tmp/.sockerless-mainpid` after `cmd.Start()`. `core.RunContainer{Pause,Unpause}ViaAgent` sends SIGSTOP/SIGCONT via `kill -<sig> $(cat <file>)` over the agent WS. Closes BUG-749.

**Phase 100 — Docker backend pod synthesis.** Docker daemon has no native pod primitive but sockerless tracks cloud containers by the `sockerless-pod` label; the Docker backend now follows the same convention. `PodList` merges Store.Pods with live Docker containers via the label filter so restarts don't drop pods. Closes BUG-754.

**Phase 101 — simulator parity for cloud-native exec/attach.** Azure sim serves `.../Microsoft.App/jobs/{job}/executions/{exec}/exec` bridged to a real `docker exec`; `core.AttachViaCloudLogs` (lifted from the existing ECS pattern) gives every FaaS backend a read-only log-streamed attach when no agent is registered. Closes BUG-760.

**Phase 102 — ECS parity via SSM.** `backends/ecs/ssm_capture.go::RunCommandViaSSM` opens an SSM session and captures stdout/stderr/exit via AgentMessage frames. `ssm_ops.go` wraps it for Export/Top/Changes/StatPath/cp/Pause using the same shell commands the FaaS reverse-agent helpers use; output goes through the same `core.Parse{Top,Stat,Changes}Output` helpers. Closes BUG-761/762.

**Audit sweep (BUG-756 → 769):**
- 756 — Lambda bootstrap user-process stdout/stderr tee'd via `io.MultiWriter` so Docker's log driver sees it. Sim's `waitAndCaptureLogs` swapped from follow-mode race to post-exit non-follow read.
- 759 — `ContainerAttach` / `ExecStart` across all 5 cloud backends now dispatch per `specs/CLOUD_RESOURCE_MAPPING.md` §Exec: agent → cloud-native (ACA console) → NotImplementedError. No silent empty streams or fake exit-126s.
- 763 — `ImageAuthProvider.OnPush` was calling `core.OCIPush` without layer data and always failing; real push now happens in `BaseServer.ImagePush` which has layer access. `ImageManager.Push` auto-fetches the cloud token via `Auth.GetToken`.
- 764 — `OCIPush` hardcoded `amd64`/`linux` + empty `config` + empty `rootfs.diff_ids`; now serialises real `Architecture` / `Os` / `Config` with `diff_ids` matching the manifest's layers.
- 765 — Lambda bootstrap + generic `--keep-alive` path publish the user PID to `/tmp/.sockerless-mainpid` so Phase 99's pause can actually find it.
- 766 — argv encoded via `:`-join broke for args containing `:`; switched to base64(JSON) through env/Dockerfile/shell.
- 767 — `sendHeartbeats` wrote `PingMessage` without the mutex `serveReverseAgent` uses for response writes — violates gorilla/websocket's Concurrency contract. Mutex now shared.
- 768 — Lambda `ContainerCreate` silently fell back to the base image when `BuildAndPushOverlayImage` failed; now fails loud.
- 769 — `ImageHistory` synthesised fake per-layer `CreatedBy` entries for images without stored history; now returns a single top-level entry.

**Spec updates.** `specs/CLOUD_RESOURCE_MAPPING.md` documents the per-backend dispatch policy, a GitLab Runner / GitHub Actions runner comparison (both strictly pull-based — the reverse-agent pattern fills the FaaS gap both sidestep), and a runner-compatibility matrix for `DOCKER_HOST=<sockerless>` configurations. Commit path documented with the find+tar mechanism and the deletion-capture limitation.

## Phases 91 / 92 / 93 / 94 / 94b — real per-cloud volumes (2026-04-21, PR #114)

`docker volume create` / `docker run -v name:/mnt` provisions real cloud storage across every backend:

- **ECS (91)** — one sockerless-owned EFS filesystem per backend (or reused via `SOCKERLESS_ECS_AGENT_EFS_ID`), one access point per Docker volume, tagged for discovery. Task defs emit `EFSVolumeConfiguration{TransitEncryption=ENABLED, AuthorizationConfig.AccessPointId}`.
- **Cloud Run (92)** — one GCS bucket per volume, labelled `sockerless-managed=true` + `sockerless-volume-name=<name>`. Jobs/Services emit `Volume{Gcs{Bucket}}`.
- **ACA (93)** — one Azure Files share per volume inside the operator's storage account, paired with a `ManagedEnvironmentsStorages` entry. Jobs/Apps emit `Volume{StorageType=AzureFile}`.
- **GCF (94)** — Functions v2's `ServiceConfig` only exposes `SecretVolumes`; volumes attach via the sanctioned escape hatch: `Services.GetService` → append `RevisionTemplate.Volumes[Gcs]` + `VolumeMounts` → `Services.UpdateService`. Partial-attach failures trigger a best-effort function delete so the create appears atomic.
- **AZF (94)** — one Azure Files share per volume; `WebApps.UpdateAzureStorageAccounts` embeds the share's plaintext access key, fetched via `StorageAccounts.ListKeys` at attach-time so rotated keys take effect without restart.
- **Lambda (94b)** — `Function.FileSystemConfigs[]` attaches EFS access points via `awscommon.EFSManager` (shared with ECS). Named volumes require Lambda-in-VPC; the backend fails loud if `SOCKERLESS_LAMBDA_SUBNETS` is empty.

Volume managers promoted into common modules (`aws-common.EFSManager`, `gcp-common.BucketManager`, `azurecommon.FileShareManager`) so FaaS backends embed them without duplication. Host-path binds (`-v /h:/c`) remain rejected across all cloud backends — no host filesystem to bind from. `Test*VolumeOperations` rewritten from NotImplemented assertions to real lifecycle assertions.

## Phase 95 — FaaS invocation-lifecycle tracker (2026-04-21, PR #114)

New `core.InvocationResult` + `Store.{Put,Get,Delete}InvocationResult` capture each FaaS invocation's exit code + finished-at + error at the source:

- Lambda maps `Invoke` result — `FunctionError` → 1, otherwise 0.
- GCF + AZF map the HTTP trigger response via `core.HTTPStatusToExitCode` (2xx → 0, 408 → 124, else 1).
- `ContainerStart` goroutine writes the result before closing the wait channel; `ContainerStop`/`ContainerKill` write `{ExitCode: 137}` (or `SignalToExitCode`) so stopped functions surface as exited even though Lambda has no invocation-cancel API.
- `CloudState.{GetContainer,ListContainers,WaitForExit}` overlay the recorded outcome on `queryFunctions` / `queryFunctionApps` and short-circuit `WaitForExit` before any cloud polling.

Crash-scoped: restart loses `InvocationResults` and falls back to cloud state (matches docker's post-daemon-crash contract). Re-enabled 7 tests that were deleted as a BUG-744 stop-gap.

## Phase 97 — GCP label-value charset compliance (2026-04-21, PR #114)

Docker labels previously serialised as a single JSON blob into a GCP label value, which GCP rejects because the charset is restricted to `[a-z0-9_-]{0,63}`. `core.AsGCPLabels` now filters values for charset + length and routes failures to `AsGCPAnnotations`. Cloud Run's cloud_state merges `Job.Annotations` / `Service.Annotations` into the label parser input. GCF (Functions v2 has no Annotations field on the Function resource) carries labels as a base64-JSON `SOCKERLESS_LABELS` env var on the function, decoded by cloud_state. `Test{CloudRun,GCF}ArithmeticWithLabels` assert the round-trip. Closes BUG-746.

## Phases 89 / 90 — stateless-backend audit + no-fakes sweep (2026-04-21, PR #113)

- **89 (stateless)** — every cloud backend derives state from cloud actuals; no on-disk state, no canonical in-memory state. `specs/CLOUD_RESOURCE_MAPPING.md` formalises the docker↔cloud mapping + restart-recovery contract. Every cloud-state callsite uses `resolve*State` helpers. `ListImages` / `ListPods` are cloud-derived. `Store.Images` disk persistence removed — cache is in-process only. Closes BUG-723–726.
- **90 (no-fakes)** — project-wide audit. Every workaround, silent substitution, and placeholder field became a bug. 11 bugs filed; 8 fixed in-sweep (729 SSM ack format, 730 synthetic image metadata, 731 NotImplemented volumes, 732 dead placeholder field, 733 fabricated PID count, 734 silent namespace substitution, 735 host-path bind rejection, 737 `SKIP_IMAGE_CONFIG` opt-out deleted); 3 scoped as dedicated phases (744 → 95, 745 → 96, 746 → 97).

## Phases 87 / 88 — Cloud Run Services + ACA Apps (2026-04-21, PR #113)

Two execution paths selected by `SOCKERLESS_GCR_USE_SERVICE` / `SOCKERLESS_ACA_USE_APP`:

- **Services (87)** — `Services.CreateService` with `INGRESS_TRAFFIC_INTERNAL_ONLY` + VPC connector + scale = 1. Peers resolve via Cloud DNS CNAMEs → `Service.Uri`. Validates VPC connector is set.
- **Apps (88)** — `ContainerAppsClient.BeginCreateOrUpdate` with internal ingress + managed environment + min/max = 1. Peers resolve via Private DNS CNAMEs → `ContainerApp.LatestRevisionFqdn`. Validates managed environment is set.

Jobs path (default) unchanged.

## Phase 86 — simulator parity + Lambda agent-as-handler (2026-04-20, PR #112)

Every cloud-API slice sockerless depends on is a first-class slice in its per-cloud simulator, validated with SDK + CLI + terraform tests (or an explicit `tests-exempt.txt` entry):

- AWS ECR pull-through cache, Lambda Runtime API (per-invocation HTTP sidecar on `host.docker.internal`), Cloud Map backed by real Docker networks.
- GCP Cloud Build + Secret Manager, Cloud DNS private zones backed by real Docker networks.
- Azure Private DNS Zones + NSG + ACR Cache Rules, managed environment backed by real Docker networks.
- Lambda agent-as-handler: `sockerless-lambda-bootstrap` polling loop + overlay-image build in `ContainerCreate` + reverse-agent WebSocket on `/v1/lambda/reverse`.

Pre-commit testing contract: every `r.Register` addition needs SDK + CLI + terraform coverage. Phase C live-AWS validated ECS end-to-end in `eu-west-1`: `docker run`, `docker run -d`, FQDN + short-name cross-container DNS, `docker exec`. 13 real bugs fixed in-branch (BUG-708–722). See [PLAN_ECS_MANUAL_TESTING.md](PLAN_ECS_MANUAL_TESTING.md).

## Stack & structure

- **Simulators** — `simulators/{aws,gcp,azure}/`, each a separate Go module. `simulators/<cloud>/shared/` for container + network helpers, `sdk-tests/` / `cli-tests/` / `terraform-tests/` for external validation.
- **Backends** — 7 backends (`backends/docker`, `backends/ecs`, `backends/lambda`, `backends/cloudrun`, `backends/cloudrun-functions`, `backends/aca`, `backends/azure-functions`). Each a separate Go module. Cloud-common shared: `backends/{aws,gcp,azure}-common/`. Core driver + shared types: `backends/core/`.
- **Agent** — `agent/` with sub-commands for the in-container driver + Lambda bootstrap. Shared simulator library: `github.com/sockerless/simulator` (aliased as `sim`).
- **Frontend** — Docker REST API. `cmd/sockerless/` CLI (zero-deps). UI SPA at `ui/` (Bun / React 19 / Vite / React Router 7 / TanStack / Tailwind 4 / Turborepo), embedded via Go `!noui` build tag. 12 UI packages across core + 6 cloud backends + docker backend + docker frontend + admin + bleephub.
- **Tests** — `tests/` for cross-backend e2e, `tests/upstream/` for external suite replays (act, gitlab-ci-local), `tests/e2e-live-tests/` for runner orchestration, `tests/terraform-integration/`, `smoke-tests/` for per-cloud Docker-backed smokes.
