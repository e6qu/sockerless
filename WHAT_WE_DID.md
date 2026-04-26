# Sockerless — What We Built

Docker-compatible REST API that runs containers on cloud backends (ECS, Lambda, Cloud Run, GCF, ACA, AZF) or local Docker. 7 backends, 3 cloud simulators, validated against SDKs / CLIs / Terraform.

See [STATUS.md](STATUS.md) for the current phase roll-up, [BUGS.md](BUGS.md) for the bug log, [PLAN.md](PLAN.md) for the roadmap, [DO_NEXT.md](DO_NEXT.md) for the resume pointer, [specs/](specs/) for architecture (start with [specs/SOCKERLESS_SPEC.md](specs/SOCKERLESS_SPEC.md), [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md), [specs/BACKEND_STATE.md](specs/BACKEND_STATE.md)).

## Post-PR-#118 audit + phase plan (PR #120 — open)

PR #118 merged the round-8 + round-9 live-AWS sweep. The post-merge audit pass (PR #120, branch `post-pr-118-bug-audit-and-phases`) records every previously-open or "known-issue" bug as a real fix in this branch, per the project's no-defer / no-fakes / no-fallbacks rule, and ships the **Phase 104 skeleton** + **Phase 105 second-wave** golden tests on the same branch.

- **18 bug closures.** BUG-802 + 638/640/646/648 + 804/806 + 820..831.
- **Phase 104 skeleton.** 13 typed driver interfaces (`backends/core/drivers_phase104.go`) — Exec / Attach / FSRead / FSWrite / FSDiff / FSExport / Commit / Build / Stats / ProcList / Logs / Signal / Registry — plus the `DriverContext` envelope, the `Driver.Describe()` composition rule, and the `SOCKERLESS_<BACKEND>_<DIMENSION>` override resolver with unit-test coverage. No behaviour change yet; per-dimension lifts follow.
- **Phase 105 second wave.** Golden shape tests for `handleLibpodInfo`, `handleLibpodContainerList`, `handleLibpodContainerRemove`. Same pattern as the first-wave `pod_inspect_shape_test.go` from BUG-804 — pin top-level shape (object vs array) plus every required field name. Third wave (network / volume / exec / events / df) queued.

**Closed in this audit** (full text in BUGS.md):

- **BUG-802** — withdrawn (round-9 measurement artifact, closed transitively by BUG-789/798).
- **BUG-638 / 640 / 646 / 648** — retroactive. Spec docs called these "known issues" (ECR push not uploading blobs, synthetic `Pushed` stream, empty-layer fallback, synthetic-metadata fallback). All four were closed by BUG-788 + Phase 90 no-fakes audit; BUGS.md rows added; `specs/IMAGE_MANAGEMENT.md` rewritten from "Known issue" to "Resolved by BUG-788".
- **BUG-804** — `podman pod inspect` libpod-shape mismatch. `api.PodInspectResponse` expanded to mirror podman's full `define.InspectPodData` (Namespace, CreateCommand, ExitPolicy, InfraContainerID, InfraConfig, CgroupParent/Path, LockNumber, RestartPolicy, BlkioWeight, CPU/Memory limits, BlkioDeviceReadBps/WriteBps, VolumesFrom, SecurityOpts, mounts, devices, device_read_bps). New schemas in `api/openapi.yaml`. Golden test in `pod_inspect_shape_test.go` asserts the response is a JSON object (never array, the original failure shape) and every libpod-shape key is present.
- **BUG-806** — `podman pod stop` Errs serialization. `[]error` can't round-trip JSON (verified against podman's own bindings). Real podman uses HTTP 409 + ErrorModel for failures and `Errs: []` on success. New `writePodActionResponse` in `handle_pods.go` emits `Errs: []` (success) or 409 + `{cause, message, response}` body (failure). `PodActionResponse.RawInput` field added.
- **BUG-820..825** — fallback / synthetic-data sweep across `backends/core`. Manifest-list silent first-fallback (820), IPAM hardcoded `172.17.0.2` (821), ImageBuild silent local-parser fallback (822), LinuxNetworkDriver netns silent-degrade (823), buildEndpointForNetwork synthetic endpoint (824), ImageRemove cloud-sync silent warn (825). Each fix surfaces the real error rather than producing fake-success.
- **BUG-826** — synthetic `exitCode: 0` in `docker stop` / `rm -f` / `restart` and `pod stop` die events. Now uses honest docker convention: SIGTERM → 143, SIGKILL → 137. Touched core + ECS + ACA + cloudrun-functions.
- **BUG-827** — Azure ContainerApps + GCP Cloud Run Jobs simulators emitted "Execution completed successfully" even when the user container failed. Now branches on `succeeded` and emits "Execution failed" on the failure path. Symmetric across both sims.
- **BUG-828** — `NetnsManager.CreateVethPair` silently ignored `ip addr add` / `ip link set up` errors, leaving half-configured veths. Now error-checked + rolls back on failure.
- **BUG-829** — `ARAuthProvider.OnRemove` and `ACRAuthProvider.OnRemove` silently `continue`d on per-tag delete failures, returning nil success while the cloud-side state diverged. Now aggregates per-tag failures and surfaces them via the BUG-825 ImageManager aggregator. Stale "non-fatal" docstrings on the AWS / GCP / Azure auth-provider hooks rewritten to match the post-BUG-825 reality.
- **BUG-830** — docker passthrough backend hardcoded `NCPU: 4, MemTotal: 8 GB` (and similar) in its `BackendDescriptor`. The `/info` handler reads real values from the daemon at request time, but if the daemon was unreachable at startup the hardcoded fallback values would be served. Fix: query the daemon's `/info` at `NewServer` with a 5-second deadline; fail startup if unavailable rather than serve fabricated capacity values.
- **BUG-831** — `ContainerCreate` and the cloud_state projections in cloudrun-functions / azure-functions / lambda seeded `EndpointSettings.IPAddress` as the literal string `"0.0.0.0"` for cloud containers without yet-routable IPs. `docker inspect` on a freshly-created container then read as if 0.0.0.0 was a real address. Fix: switch the seed to `""` across all 7 backends; the two-phase service-discovery detection in `aca`/`cloudrun`/`service_discovery_cloud.go` already accepts both as the unresolved-IP sentinel, so existing logic still works on state recovered from older registries.
- **BUG-826 (extension)** — round-9's BUG-826 fix landed in core / ecs / aca / cloudrun-functions but missed the cloudrun and azure-functions backends. Same SIGTERM=143 / SIGKILL=137 fix shape applied to bring all 7 backends to docker-convention parity.

**New phases queued** ([PLAN.md](PLAN.md)):

- **Phase 106** — Real GitHub Actions runner integration via `actions/runner` + DOCKER_HOST → sockerless. ECS+Lambda first; rest gated on Phase 104. Canonical workload sweep (matrix builds, services, artifacts, secrets, fail-fast, log streaming).
- **Phase 107** — Real GitLab Runner docker-executor → sockerless. Same coverage shape; `dind` sub-test included; Kubernetes-executor follow-up under Phase 104.
- **Phase 108** — Cross-simulator feature parity audit. Walk every cloud-API call sockerless makes; build aws/gcp/azure parity matrix; close every gap in-phase per the no-defer rule.

Phase 105 reframed as "rolling — first wave landed" since BUG-804/806 are now closed.

## Round-9 manual-test ↔ spec crosswalk (closed via PR #118)

Per-test walk through [PLAN_ECS_MANUAL_TESTING.md](PLAN_ECS_MANUAL_TESTING.md) cross-referenced against [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md). Each test ran one at a time on live ECS + Lambda; results archived in [docs/manual-test-spec-crosswalk.md](docs/manual-test-spec-crosswalk.md).

ECS Tracks A/B/C/E/F/G/I (~80 tests) + Lambda Track D (D1-D9). 14 bugs filed and fixed (BUG-801, 803, 805, 813, 789/798, 815, 816, 817, 795, 818, 819 + the Lambda set 807, 808, 809, 810, 811, 812). Live AWS torn down post-merge; per-cloud terragrunt sweep (BUG-819 fix) makes destroys self-sufficient.

## Round-8 live-AWS sweep — 13 bugs fixed (this branch — pending PR #118)

Two-round live-AWS test sweep against `eu-west-1` ECS + Lambda. 278 tests across both rounds, 13 bugs filed + fixed. Two open follow-ups (BUG-789/798 SSM frame parsing, BUG-795 podman pod-list filter).

Headline fixes:
- **BUG-786 — `docker rmi <tag>` reappearing.** `StoreImageWithAliases` puts the image value under multiple keys; partial-untag only updated the "remaining" tag entries. Fix sweeps every `Store.Images` entry whose `Value.ID` matches and rewrites it. New `StateStore.Entries()` exposes the snapshot under-lock.
- **BUG-788 — registry-to-registry layer mirror.** `ImagePull` now downloads layer blobs into `Store.LayerContent` and records `[]ManifestLayerEntry` per image; `OCIPush` uses source compressed digests verbatim. Verified live: `docker pull public.ecr.aws/.../alpine && docker tag … && docker push <ecr>` → `Pushed`.
- **BUG-790 — sync `docker stop`.** New `waitForTaskStopped` blocks until ECS observes STOPPED; immediate `docker rm` succeeds. Synthesised `container.die exitCode=0` event removed; real die event comes from cloud-state poller. Closes BUG-796 transitively.
- **BUG-794 — cross-network isolation.** Per-network SG is the sole authority for containers with `--network X`; default SG only applies to networkless containers.
- **BUG-799 — recovery skip STOPPED.** `ScanOrphanedResources` no longer treats STOPPED tasks as active orphans.
- **BUG-800 — stateless invariant restored.** `core.ResourceRegistry` Save/Load collapsed to no-ops; 11 stale `sockerless-registry.json` files swept from the tree.
- **BUG-787 — spec-doc refresh.** `specs/CLOUD_RESOURCE_MAPPING.md` now reflects landed Phases 91–94, 96, 102; new "Acceptable gaps" section formalises maintainer-approved exceptions (ECS commit, ECS pause-without-bootstrap, ContainerResize/ExecResize, ImageSave/Search, streaming stats, ContainerTop without exec, ContainerExport, rename divergence).

Smaller: BUG-791 (`handleGetArchive` → `WriteError`), BUG-792 (commit error stripped of phase ref), BUG-793 (terraform attaches `AWSLambdaVPCAccessExecutionRole`), BUG-797 (Lambda public.ecr.aws short-circuit). Code-side gap matching: BaseServer.ContainerResize/ExecResize/ImageSave/ImageSearch return clean NotImpl on cloud backends; streaming `docker stats` returns NotImpl when CloudState is set; local docker backend keeps full functionality via overrides. Teardown self-sufficiency: `aws_ecr_repository.force_delete = true` + ECS module's destroy-time task-def deregister sweep — `terragrunt destroy` succeeds without manual cleanup.

## Round-7 live-AWS sweep — 16 bugs fixed (PR #117, 2026-04-25)

BUG-770..785 closed across one round of live-AWS testing. Categories: ImageRemove correctness, ECS task lifecycle (rename, restart, kill-signal mapping, removal-via-registry), libpod compat (specgen create, container list, normalised times), OCI push auth + config-blob correctness, lambda bootstrap PID publishing + heartbeat mutex, registry persistence robustness. See git log for individual fixes.

## Closed phases — narrative

Newest first; older entries deliberately compressed (full detail in `git log` and BUGS.md).

### Phases 96 / 98 / 98b / 99 / 100 / 101 / 102 + 13-bug audit sweep (PR #115, 2026-04-24)

Reverse-agent + SSM machinery for every dimension docker exposes that needs in-container access:

- **96** — shared `core.ReverseAgentRegistry` + `HandleReverseAgentWS` + `ReverseAgent{Exec,Stream}Driver`. CR/ACA/GCF/AZF mount `/v1/<backend>/reverse` and inject `SOCKERLESS_CALLBACK_URL` so the bootstrap dials back. Closes BUG-745.
- **98** — agent-driven `docker top / stat / get-archive / put-archive / export / diff` via `core.RunContainer*ViaAgent` (ps/stat/tar/find over the WS). Wired across every FaaS backend. Closes BUG-750/751/752/753.
- **98b** — agent-driven `docker commit` (opt-in via `SOCKERLESS_ENABLE_COMMIT=1`): `find / -xdev -newer /proc/1` + `tar` over the agent → diff layer stacked on the source image's rootfs. Deletions not captured — documented limitation, addressed by Phase 103.
- **99** — `docker pause` / `unpause` via SIGSTOP/SIGCONT over the agent, using the bootstrap-published `/tmp/.sockerless-mainpid`. Closes BUG-749.
- **100** — Docker backend pod synthesis via the shared `sockerless-pod` label convention. Closes BUG-754.
- **101** — sim parity for cloud-native exec/attach + read-only log-streamed attach fallback for FaaS. Closes BUG-760.
- **102** — ECS parity for filesystem-ops + pause/unpause via SSM ExecuteCommand (`backends/ecs/ssm_capture.go::RunCommandViaSSM` + `ssm_ops.go`). Output goes through the same `core.Parse{Top,Stat,Changes}Output` helpers as the FaaS path. Closes BUG-761/762.

13-bug audit sweep (756–769) cleaned up dispatch policy, OCI push correctness, argv encoding, PID publishing, heartbeat mutex serialization, overlay-build hard-fail (no silent fallback to base image), and `ImageHistory` fake-text removal.

### Phases 91 / 92 / 93 / 94 / 94b — real per-cloud volumes (PR #114, 2026-04-21)

`docker volume create` / `docker run -v name:/mnt` provisions real cloud storage on every backend: ECS+Lambda → EFS access points (shared `aws-common.EFSManager`), CR+GCF → GCS buckets, ACA+AZF → Azure Files shares. Host-path binds remain rejected (no host filesystem in the cloud). Closes BUG-735/736/748.

### Phase 95 — FaaS invocation-lifecycle tracker (PR #114)

`core.InvocationResult` + `Store.{Put,Get,Delete}InvocationResult` capture per-container exit code + finished-at + error at the invocation source. Lambda → `Invoke` result; GCF/AZF → HTTP trigger response via `core.HTTPStatusToExitCode`. CloudState overlays the recorded outcome on `queryFunctions`. Closes BUG-744; re-enabled 7 tests deleted as the original stop-gap.

### Phase 97 — GCP label-value charset compliance (PR #114)

Charset-safe label encoding via `core.AsGCPLabels` + annotation routing for non-conforming values; GCF carries labels as base64-JSON env. Closes BUG-746.

### Phases 89 / 90 — stateless audit + no-fakes sweep (PR #113, 2026-04-21)

- **89** — every cloud backend derives state from cloud actuals. `resolve*State` cache+cloud-fallback helpers, cloud-derived `ListImages` / `ListPods`, `Store.Images` disk persistence removed. Closes BUG-723–726. (BUG-800 in round-8 caught a residual `ResourceRegistry` persistence and finished the job.)
- **90** — project-wide audit of workarounds, silent substitutions, placeholder fields. 11 bugs filed; 8 fixed in-sweep (729 SSM ack, 730 synthetic image metadata, 731 NotImpl volumes, 732 dead placeholder, 733 fabricated PIDs, 734 silent namespace substitution, 735 host-path bind rejection, 737 `SKIP_IMAGE_CONFIG` opt-out deleted); 3 → dedicated phases (744 → 95, 745 → 96, 746 → 97).

### Phases 87 / 88 — Cloud Run Services + ACA Apps (PR #113)

Two execution paths selected by `SOCKERLESS_GCR_USE_SERVICE` / `SOCKERLESS_ACA_USE_APP`. Services/Apps create internal-ingress workloads with VPC connector / managed environment; peers resolve via Cloud DNS / Private DNS CNAMEs. Jobs path (default) unchanged. Closes BUG-715, 716.

### Phase 86 — simulator parity + Lambda agent-as-handler (PR #112, 2026-04-20)

Every cloud-API slice sockerless depends on is a first-class slice in its per-cloud simulator, validated with SDK + CLI + terraform tests. AWS ECR pull-through cache, Lambda Runtime API, Cloud Map; GCP Cloud Build + Secret Manager, Cloud DNS; Azure Private DNS + NSG + ACR Cache Rules, managed environment. Lambda agent-as-handler: `sockerless-lambda-bootstrap` polling loop + overlay-image build in `ContainerCreate` + reverse-agent WS at `/v1/lambda/reverse`. Pre-commit contract: every new sim handler needs SDK+CLI+terraform coverage. Phase C validated ECS end-to-end in `eu-west-1`. Closes BUG-708–722.

## Stack & structure

- **Simulators** — `simulators/{aws,gcp,azure}/`, separate Go modules. `simulators/<cloud>/shared/` for container + network helpers; `sdk-tests/` / `cli-tests/` / `terraform-tests/` for external validation.
- **Backends** — 7 backends (`backends/docker`, `backends/ecs`, `backends/lambda`, `backends/cloudrun`, `backends/cloudrun-functions`, `backends/aca`, `backends/azure-functions`). Each a separate Go module. Cloud-common shared: `backends/{aws,gcp,azure}-common/`. Core driver + shared types: `backends/core/`.
- **Agent** — `agent/` with sub-commands for the in-container driver + Lambda bootstrap. Shared simulator library: `github.com/sockerless/simulator`.
- **Frontend** — Docker REST API. `cmd/sockerless/` zero-dep CLI. UI SPA at `ui/` (Bun / React 19 / Vite / React Router 7 / TanStack / Tailwind 4 / Turborepo), embedded via Go `!noui` build tag.
- **Tests** — `tests/` for cross-backend e2e, `tests/upstream/` for external-suite replays (act, gitlab-ci-local), `tests/e2e-live-tests/` for runner orchestration, `tests/terraform-integration/`, `smoke-tests/` for per-cloud Docker-backed smokes.
