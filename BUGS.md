# Known Bugs

**728 total — 728 fixed, 0 open, 0 false positives.**

For narrative context see [WHAT_WE_DID.md](WHAT_WE_DID.md) and [PLAN.md](PLAN.md). Architecture-level state derivation is documented in [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md) and [specs/BACKEND_STATE.md](specs/BACKEND_STATE.md).

Standing workflow rule: every CI / live-cloud failure lands here with a short root-cause trace before it's fixed.

## Fixed

| ID | Sev | Area | Summary |
|----|-----|------|---------|
| 728 | H | sim/aws | ECS exec WebSocket emitted raw bytes; backend's SSM decoder saw empty output. Simulator now builds real SSM AgentMessage frames (`output_stream_data`, exit-code PayloadType=12, `channel_closed`) — see `simulators/aws/ssm_proto.go`. |
| 727 | H | ci | Unit job panicked on nil `dockerClient` in cloudrun/aca/gcf/azf arithmetic integration tests. CI `test` job now sets `SOCKERLESS_INTEGRATION=1` + verifies docker; TestMain CI guard fails loud if env var missing. |
| 726 | H | aca/ecs/cloudrun | NetworkState (SG, Cloud Map namespace, DNS zone, NSG) was in-memory only. `resolveNetworkState` now derives from cloud actuals by deterministic name. |
| 725 | H | aca/cloudrun/ecs/lambda | Per-backend state maps were the canonical lookup for 16 cloud callsites; restart-unsafe. All callsites migrated to `resolve*State` helpers (cache + cloud-derived fallback). |
| 724 | M | aca/cloudrun/ecs | Docker pods (libpod) were tracked in `Store.Pods` only. `BaseServer.PodList` now merges `CloudPodLister` results; each backend groups cloud resources by `sockerless-pod` tag. |
| 723 | H | all-cloud | `Store.Images` disk persistence removed; `docker images` now cloud-derived across 6 cloud backends via `core.OCIListImages` (Artifact Registry / ACR) + ECR SDK (ECS/Lambda). |
| 722 | H | ecs | Restart lost `ECSState.TaskARN`; `cloudExecStart` returned ASCII error through hijacked conn → docker CLI reported `unrecognized stream: 110`. `resolveTaskARN` lazy-recovery via ListTasks + tag filter. Later subsumed by BUG-725. |
| 721 | M | ecs | SSM agent retransmits `output_stream_data` until it sees an ack it accepts; sockerless's ack format isn't yet recognised. Pragmatic dedupe by MessageID UUID in `ssmDecoder`. Proper ack-acceptance still outstanding as follow-up work. |
| 720 | H | ecs | Task IAM role missing `ssmmessages:*` permissions → ECS Exec data channel closed immediately. Terraform module adds `ECSExecSSMMessages` statement. |
| 719 | H | ecs | `RunTask` omitted `EnableExecuteCommand: true` → exec fails post-launch. Fixed in `startTask`. |
| 718 | H | lambda | Cross-cloud sibling of BUG-708 + silent `pushToECR` fallback. Same credential-ARN wiring; fallback removed. |
| 717 | H | ecs | `docker exec` returned `unrecognized stream: 69` because SSM binary frames were passed through as Docker mux bytes. Full SSM AgentMessage parser + ack writer + stdin wrapping in `backends/ecs/ssm_proto.go`. |
| 716 | H | aca | Private DNS A-records got the `0.0.0.0` placeholder (ACA Jobs have no addressable per-execution IP). Closed via Phase 88: `SOCKERLESS_ACA_USE_APP=1` switches to ContainerApps with internal ingress; peer discovery writes CNAMEs to `LatestRevisionFqdn`. |
| 715 | H | cloudrun | Same symptom as BUG-716 on GCP. Closed via Phase 87: `SOCKERLESS_GCR_USE_SERVICE=1` + `SOCKERLESS_GCR_VPC_CONNECTOR` switches to Cloud Run Services with internal-only ingress; peer discovery writes CNAMEs to `Service.Uri`. |
| 714 | H | ecs | Cloud Map A-record registration used the `0.0.0.0` placeholder from in-memory state. Registration moved after `waitForTaskRunning` so the real Fargate ENI IP is available (via `extractENIIP`). |
| 713 | H | cloudrun | `ManagedZones.Create` non-idempotent — 409 conflict left the network unusable. Catch 409, fall back to `ManagedZones.Get`, cache. |
| 712 | H | ecs | `cloudNetworkCreate` + `cloudNamespaceCreate` non-idempotent — retries crashed on duplicate SG / namespace. Both now reuse existing resources by name lookup. |
| 711 | H | ecs | `DnsSearchDomains` rejected by Fargate on awsvpc. `buildContainerDef` wraps argv in `/bin/sh -c` that rewrites `/etc/resolv.conf` then `exec`s original argv with POSIX-quoted args (`shellQuoteArgs`). |
| 710 | M | cli/all-backends | Default port `:2375` collides with Docker/Podman daemons. Changed to `:3375` everywhere (CLI + 7 backends + docs). Pre-commit hook `scripts/check-port-defaults.sh` locks it in. |
| 709 | H | ecs | `waitForOperation` polled Cloud Map's `GetOperation` without sleeping — burned 60 API calls in <10s while real provisioning takes ~30-60s. `pollOperation` helper with 2s sleep, 60× budget. |
| 708 | L | ecs | ECR pull-through docker-hub rules require a Secrets Manager credential ARN. `SOCKERLESS_ECR_DOCKERHUB_CREDENTIAL_ARN` now wired through with an explicit error (no silent fallback) when unset. |
| 707 | M | sim/gcp | Cloud Build Secret Manager integration — `AvailableSecrets.SecretManager` populated from opts; simulator resolves `projects/P/secrets/S/versions/V` references via new `simulators/gcp/secretmanager.go`. |
| 706 | M | sim/azure + aca | ACR cache-rule CRUD added to simulator; `backends/azure-common/ResolveAzureImageURIWithCache` rewrites Docker Hub refs through the configured ACR. |
| 705 | H | sim/aws | Lambda bypassed the real Runtime API. `simulators/aws/lambda_runtime.go` implements the per-invocation HTTP sidecar (`GET /invocation/next`, `POST /invocation/{id}/response`, etc.) with full Lambda env + `host.docker.internal` wiring. |
| 704 | M | sim/gcp | Cloud Build slice — CreateBuild LRO, source tarball extraction, Docker build steps, SUCCESS/FAILURE/CANCELLED state machine matching the real API. |
| 703 | H | aca | NSG integration — real `armnetwork.SecurityGroupsClient` via `ClientFactory`; simulator grew securityRules sub-resource. |
| 702 | H | aca | Private DNS Zones integration — `armprivatedns.RecordSetsClient` + per-network `skls-<name>.local` zone; in-memory `serviceRegistry` removed. |
| 701 | H | sim/{aws,gcp,azure} | Each simulated task/job ran as a standalone host-Docker container with no shared user-defined network — cross-container DNS broken. Each cloud's DNS slice now creates a per-namespace Docker network and connects tasks by service-name alias. |
| 700 | M | aca/cloudrun/ecs | `docker network create` silently lost cloud-side failures. `NetworkCreate` response now populates `Warning` with a semicolon-separated list. |
| 699 | M | sim/aws | EC2 didn't pre-register `subnet-sim` — Cloud Map namespace create failed VPC-ID resolution. Startup now auto-creates `vpc-sim` + `subnet-sim` idempotently. |
| 698 | C | core | `docker run -d` hung because the wait handler blocked on `WaitForExit` before committing headers — docker CLI never unblocked its `/wait` call to issue `/start`. Early `flushWaitHeaders` commits the 200 before blocking. |
| 697 | M | core | `docker pull` state didn't survive backend restart. Originally fixed with disk persistence; later (BUG-723) swapped for cloud-derived `ListImages`. |
| 696 | M | sim/aws | ECR pull-through-cache APIs missing. CRUD + real AWS error shapes added. |
| 695 | H | core | `StreamCloudLogs` rejected `created`-state containers unconditionally — broke create→attach→start flow. `AllowCreated` option added. |
| 694 | H | core | `StreamCloudLogs` follow loop exited on `!running` — `created` state is also non-running. Switched to `isTerminalState`. |
| 693 | H | ecs | Task definition used raw unqualified image ref — Fargate can't pull. Ported Lambda's `resolveImageURI` to ECS. |
| 692 | C | ecs | `docker run` hung after POST /containers/create — `ContainerAttach` delegated to `BaseServer` which returned an EOF pipe. ECS-specific attach now streams CloudWatch logs. |
| 691 | M | sim/gcp | Smoke test long-running container showed empty `docker ps` — same root cause as BUG-688. |
| 690 | M | sim/gcp | `docker stop` returned 304 for running containers — same root cause as BUG-688. |
| 689 | M | sim/gcp | Short-lived containers missed log output — `waitAndCaptureLogs` now waits for drain channel before returning. |
| 688 | H | sim/gcp | `docker ps` showed running container as not running — GCP simulator missing GET /executions/{id} endpoint. |
| 687 | H | sim/cloudrun | Empty `docker logs` — sim tried to pull cloud registry URI locally. `ResolveLocalImage` now maps AR/ECR/ACR URIs back to Docker Hub. |
| 686 | H | sim/all | Workloads ran via `os/exec` instead of real containers — exec/archive/fs ops impossible. Migrated to Docker SDK. |
| 685 | H | sim/azure | ContainerAppJob missing SystemData — creation time unavailable to CloudState. |
| 684 | H | sim/gcp | Job missing `LatestCreatedExecution` — execution state undeterminable. Added ExecutionReference + CompletionTime updates. |
| 683 | H | ecs/cloudrun/aca | Auto-agent (local process spawn + Store reads) violated stateless invariant. Removed from all cloud backends. |
| 682 | H | gcf/cloudrun | GCP label value 63-char limit truncated 64-char container IDs. Full ID moved to annotations; env var for GCF. |
| 681 | H | cloudrun/aca/gcf/azf | CloudState implementations were stubs reading Store.Containers. Replaced with real ListJobs / ListFunctions queries. |
| 680 | M | core | Handler files used `Store.ResolveContainerID` directly — failed in stateless mode. Migrated to `ResolveContainerAuto` / `ResolveContainerIDAuto`. |
| 679 | M | core | `StreamCloudLogs` follow-mode checked `Store.Containers.Get` — fails in stateless mode. Uses `ResolveContainerAuto`. |
| 678 | M | cloudrun | `ContainerUpdate` delegated to BaseServer without resolving container first. Added auto-resolve. |
| 677 | M | cloudrun | `ContainerTop` both branches delegated identically to BaseServer. Returns NotImplemented when no agent connected. |
| 676 | H | core | `NetworkConnect/Disconnect` used `Store.ResolveContainerID`. Switched to `ResolveContainerIDAuto`. |
| 675 | H | core | `ExecCreate` used `Store.ResolveContainerID` + `Store.Containers.Get` — fails in stateless mode. Uses `ResolveContainerAuto`. |
| 674 | H | core | `ContainerStart/Stop/Kill/Remove/Restart/Logs/Wait/Attach/Stats/Rename/Pause/Unpause` all used `Store.ResolveContainerID`. Switched to `ResolveContainerAuto`. |
| 673 | H | core | `ContainerUpdate` used `Store.ResolveContainerID` + `Store.Containers.Update`. Uses `ResolveContainerIDAuto`. |
| 672 | H | core | `ContainerTop` used `Store.ResolveContainerID` + `Store.Containers.Get`. Uses `ResolveContainerAuto`. |
| 671 | H | core | `ContainerList` used `Store.Containers.List` → empty on cloud backends. Uses `CloudState.ListContainers` when available. |
| 670 | H | core | `ContainerInspect` used `Store.ResolveContainer`. Uses `ResolveContainerAuto`. |
| 669 | H | ecs | `ContainerLogs` fell back to `BaseServer.ContainerLogs` when taskID unknown — stateless violation. Returns clear error instead. |
| 668 | H | all-cloud | `StreamCloudLogs` used `Store.ResolveContainerID` + `Store.Containers.Get` — docker logs 404 on all cloud backends. Uses `ResolveContainerAuto`. |
| 667 | H | lambda | Missing CloudStateProvider — docker ps / inspect / stop broken after `PendingCreates.Delete`. `lambdaCloudState` queries `ListFunctions` + tags. |
| 666 | L | ecs | `docker run -d` blocked ~30s on ECS provisioning. Async wait for RUNNING — RunTask returns immediately, poll in background. |
| 665 | M | ecs | `docker logs` 404 after restart — task ID only in local state. Cloud query by container ID tag when local state missing. |
| 664 | H | ecs | CloudState queried only RUNNING tasks. Now queries RUNNING + STOPPED. |
| 663 | H | ecs | `docker wait` hung in auto-agent mode. Checks local `WaitChs` first. |
| 662 | H | ecs | Auto-agent delegated to `BaseServer.ContainerStart` which reads Store. Broke stateless invariant. |

Earlier phases (≤ BUG-661) — one-liners per historical bug kept in `git log` + `specs/` specs. Summaries too terse to be useful here.

## Open

*(none)*

## False positives

*(none)*

## Cross-cloud sweep notes

When a bug is found, similar code paths in other backends / clouds / simulators are checked too. Notable sweep outcomes:

- BUG-708 ECS-only — Azure uses ACR cache rules with managed-identity auth; GCP uses Artifact Registry pull-through (different mechanism).
- BUG-709 ECS-only — Azure ACA's SDK helper sleeps internally; GCP's SDK handles polling.
- BUG-710 swept all 7 backend mains + CLI + READMEs + example terraform + `tools/http-trace`.
- BUG-711 ECS-only — no other backend sets explicit DNS search domains; GCP/Azure rely on per-network DNS zones with FQDN resolution.
- BUG-712 → found BUG-713 in cloudrun (same non-idempotent-create pattern). Azure ACA's `BeginCreateOrUpdate` is PUT-style and idempotent; Lambda/GCF/AZF have no cloud-side network creation.
- BUG-714 → found BUG-715 (cloudrun) and BUG-716 (aca) with the same placeholder-IP symptom. All three backends seed `ep.IPAddress = "0.0.0.0"` at create time; each needed a different structural fix (ECS got the real Fargate ENI IP; cloudrun + aca moved to Services/Apps with stable FQDNs in Phase 87/88).
