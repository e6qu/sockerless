# Known Bugs

**846 total — 846 fixed, 0 open, 1 false positive.** Three sections: [Open](#open) / [False positives](#false-positives) / [Resolved](#resolved). Per-project rule: no bug deferral, no fakes, no fallbacks — every filed bug ships a fix in the same round. Fix detail beyond the one-liner: see `git log <commit>` or the linked PR.

For narrative context see [WHAT_WE_DID.md](WHAT_WE_DID.md) and [PLAN.md](PLAN.md). Architecture-level state derivation is in [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md) and [specs/BACKEND_STATE.md](specs/BACKEND_STATE.md).

Standing workflow rule: every CI / live-cloud failure lands here with a short root-cause line before it's fixed. Workarounds, fakes, placeholders, silent fallbacks, and incomplete implementations are all bugs and get the same treatment.

## Open

_(none — every filed bug ships a fix in the same round per the no-defer rule)_

## False positives

Findings that look like bugs under the "no fakes, no workarounds" principle but are legitimate by design. Listed so future audits don't re-flag them.

| Area | Finding | Why it's not a bug |
|------|---------|--------------------|
| `backends/aca/azure.go::fakeCredential` | Returns a literal `"fake-token"` as bearer token against simulator endpoints. | Simulators intentionally don't verify bearer tokens — token verification requires a real Azure AD endpoint that the sim doesn't emulate. The credential is only wired via `newAzureClientsWithEndpoint` (the simulator path); production uses `azidentity.NewDefaultAzureCredential`. |

## Resolved

One-liner per bug. Latest first; pre-Round-7 entries are extra-terse (full detail in `git log` / PR descriptions).

### Phase 110 prep (PR #122 — open) (BUG-845..846)

| ID | Sev | Area | One-liner |
|----|-----|------|-----------|
| 845 | M | terraform | `terraform/environments/lambda/live/terragrunt.hcl` was pinned to `us-east-1` + bucket `sockerless-terraform-state` + dynamodb_table — drift from the ECS live env (`eu-west-1` + `sockerless-tf-state`). `manual-tests/01-infrastructure.md` documents Lambda reusing the ECS subnets + security group, which is impossible cross-region. Fix: realign Lambda live env to `eu-west-1` + `sockerless-tf-state`; mirror the ECS env's `provider "aws"` generate block; drop the dynamodb_table reference (ECS env doesn't use one). |
| 846 | M | core (image resolve) + docs | Live AWS test pass against an empty account fails immediately on first `docker run alpine:latest` because the ECS + Lambda backends previously routed Docker Hub library refs through an ECR pull-through cache rule with `registry-1.docker.io` upstream — and AWS rejects that without a Secrets Manager-backed Docker Hub PAT. Fix: drop the Docker Hub PAT path entirely (the project's no-credentials-on-disk discipline avoids it by design). Sockerless's `resolveImageURI` now rewrites Docker Hub library refs (`alpine`, `node:20`, `nginx:alpine`) to the AWS Public Gallery mirror at `public.ecr.aws/docker/library/<name>:<tag>`, which Fargate pulls directly with no credentials. Lambda routes the same refs through an ECR pull-through cache pointed at `public.ecr.aws` (Lambda needs single-arch ECR-hosted images, but the cache still doesn't need credentials since it caches public images). Other public registries (`ghcr.io`, `quay.io`, `registry.k8s.io`, `mcr.microsoft.com`) get their own pull-through cache rules — also no credentials. Docker Hub user/org refs (`myorg/myapp`) are rejected with a clear error pointing the operator at `docker push <ecr-uri>`. The `SOCKERLESS_ECR_DOCKERHUB_CREDENTIAL_ARN` env var and the equivalent terraform variable are gone. `manual-tests/01-infrastructure.md` documents the new "Image source policy" section (AWS Public Gallery / ECR pull-through / operator-owned ECR — three credential-free routes). |

### PR #120 — typed-driver migration CI pass (BUG-836..844)

| ID | Sev | Area | One-liner |
|----|-----|------|-----------|
| 836 | H | sim/aws | ECS task lifecycle skipped real container start when no awslogs configured (synthetic RUNNING-forever). Fix: container starts unconditionally; `discardLogSink` carries the path when no log driver. |
| 837 | L | tests/aws-sdk | `TestECS_TagResource_RejectsStoppedTask` flaked under CI podman contention with a fixed 8s sleep. Fix: replaced with a 60s poll-for-STOPPED loop. |
| 838 | M | sim/gcp | `ServiceV2.Ingress` typed `string` couldn't decode the Go REST client's numeric proto-JSON enum form. Fix: introduced an `enumString` type that accepts both numeric and string forms, round-trips as string. |
| 839 | M | sim/azure | Every site shared `r.Host` as DefaultHostName so multi-site routing collided. Fix: per-site `<name>.azurewebsites.net`; invoke handler routes by Host header. |
| 840 | L | tests/e2e | `TestImageBuild` asserted on the synthetic local-build path BUG-822 removed. Fix: rewrote to assert the 501 + `cloud build service` error contract. |
| 841 | H | ecs | `handleContainerKill` routed every signal through SignalDriver; `ssmSignalDriver` only handles SIGSTOP/SIGCONT. Fix: branch on signal — pause/cont via SSM, everything else via legacy `ContainerKill` (`ECS.StopTask`). |
| 842 | H | sim/aws | SSM session ignored `input_stream_data` AgentMessage frames so binary stdin (tar archives) never reached the user process. Fix: decode AgentMessage frames; forward `input_stream_data` payload to exec stdin; close stdin on FIN. |
| 843 | M | ecs | `ContainerPutArchiveViaSSM` didn't pre-create destination directory; `tar -xf -C <missing>` failed. Fix: `mkdir -p <path> && tar -xf - -C <path>`. |
| 844 | H | sim/aws | `RunTask` returned hardcoded `subnet-sim00001` and `10.0.x.x` IPs ignoring the request. Fix: `AllocateSubnetIP(subnetID)` reads the registered subnet's CidrBlock; 400 on unknown subnet. |

### PR #120 audit pass — fallback / synthetic-data findings (BUG-820..835)

| ID | Sev | Area | One-liner |
|----|-----|------|-----------|
| 820 | H | core (registry) | `FetchImageMetadata` for multi-arch fell back to `Manifests[0]` when no `linux/amd64` entry. Fix: drop fallback; return error listing every available os/arch. |
| 821 | M | core (ipam) | `IPAllocator.AllocateIP` returned hardcoded `172.17.0.2/16` when networkID wasn't registered (default bridge). Fix: register bridge subnet on init; unknown-network returns zero values. |
| 822 | H | core (build) | `ImageManager.Build` silently fell back to local Dockerfile parser (drops RUN steps) when no cloud build configured. Fix: return `NotImplementedError`; `SOCKERLESS_LOCAL_DOCKERFILE_BUILD=1` opt-in for parse-only. |
| 823 | H | core (network) | `LinuxNetworkDriver.Create/Connect` silently continued in synthetic-only mode on netns failure. Fix: roll back + return real error; `SOCKERLESS_NETNS_BEST_EFFORT=1` opt-in for degradation. |
| 824 | M | core (network) | `buildEndpointForNetwork` synthesised `172.17.0.<n>` for unknown networks, colliding with bridge allocations. Fix: returns nil; callers skip nil endpoints so `network not found` surfaces honestly. |
| 825 | M | core (registry) | `ImageManager.Remove` silently logged warnings when cloud-side delete failed. Fix: aggregate errors into `ServerError` listing them so operators can rerun rmi. |
| 826 | M | core / all-cloud | `docker stop` / `rm -f` / `restart` / `pod stop` emitted synthetic `container.die` with `exitCode=0` regardless of how the container died. Fix: 143 (SIGTERM) / 137 (SIGKILL) per `core.SignalToExitCode`; persisted via `Store.ForceStopContainer`. |
| 827 | M | sim/azure + sim/gcp | Job log streams emitted `"Execution completed successfully"` even when user container exited non-zero. Fix: branch on `succeeded` flag — `"Execution failed"` on failure. |
| 828 | H | core (network) | `NetnsManager.CreateVethPair` ignored errors from `ip addr add` / `ip link set up`. Fix: each step error-checked, rolls back on first failure. |
| 829 | M | gcp/azure-common (auth) | `ARAuthProvider.OnRemove` / `ACRAuthProvider.OnRemove` silently `continue`d on per-tag delete failures. Fix: collect failures into combined error so `ImageManager.Remove` aggregator surfaces them. |
| 830 | M | docker | Hardcoded `NCPU: 4, MemTotal: 8 GB, OSType: "linux"` in `BackendDescriptor` survived as fallback when daemon unreachable. Fix: query daemon `/info` at startup with 5s deadline; fail startup on unavailable. |
| 831 | L | core / all-cloud | `EndpointSettings.IPAddress` seeded as literal `"0.0.0.0"` for cloud containers without a routable IP — read as real address in `docker inspect`. Fix: empty string instead. |
| 832 | M | sim/aws | (Phase 108) ECS missing `TagResource` / `UntagResource` handlers — backend tag writes silently 404'd. Fix: added both handlers; STOPPED-task gate mirrors real ECS. |
| 833 | H | sim/gcp | (Phase 108) Cloud Run v2 Services routes missing — `UseService` mode 404'd against sim. Fix: `simulators/gcp/cloudrunservices.go` covers v2 Create/Get/List/Update/Delete. |
| 834 | H | sim/azure | (Phase 108) ContainerApps Apps surface entirely absent — `UseApp` mode 404'd. Fix: `simulators/azure/containerapps_apps.go` covers PUT/GET/LIST/DELETE on `Microsoft.App/containerApps`. |
| 835 | M | sim/azure | (Phase 108) `WebApps.UpdateAzureStorageAccounts` handler missing — function-app named volumes failed at start. Fix: added handler + symmetric `GET .../config/azurestorageaccounts/list`. |

### Round-9 (PR #118) live-AWS sweep (BUG-801..819 + retroactives)

| ID | Sev | Area | One-liner |
|----|-----|------|-----------|
| 801 | L | ecs | `docker inspect` returned `HostConfig.Memory: 0` / `NanoCpus: 0`. Fix: `taskToContainer` parses task.Memory (MB) and task.Cpu (1024-shares) into bytes / nanoCPUs. |
| 803 | L | docs | `specs/CLOUD_RESOURCE_MAPPING.md` matrix vs §Notes inconsistent for `ContainerExport`. Fix: aligned to "⚠ via SSM" for ECS and "⚠ agent only" for FaaS+CR+ACA. |
| 805 | M | ecs | `docker stop` / `pod stop` timed out at 60s on Fargate STOPPING transitions. Fix: default `stopTimeout` 60→120s; user-supplied gets 60s grace instead of 30s. |
| 807 | H | lambda | `docker run` returned `ResourceConflictException: Pending` — backend invoked the function before AWS finished VPC ENI provisioning. Fix: invoke goroutine waits on `awslambda.NewFunctionActiveV2Waiter` (5-min cap) before `Invoke`. |
| 808 | H | lambda | `SOCKERLESS_LAMBDA_PREBUILT_OVERLAY_IMAGE` was dead code without `SOCKERLESS_CALLBACK_URL`. Fix: split the switch — prebuilt-image alone routes user CMD via `SOCKERLESS_USER_CMD/ENTRYPOINT`. |
| 809 | H | core | `docker exec` against backend returning NotImplementedError emitted `unrecognized stream: 100` because handler hijacked + sent `101 UPGRADED` before calling backend. Fix: call ExecStart first, return `WriteError` on failure, only hijack on success. |
| 810 | L | core | Startup log `loaded resource registry from disk` survived after BUG-800 made `Registry.Load()` a no-op. Fix: dropped dead call, reworded log line. |
| 811 | M | lambda | After backend restart, every Lambda function reported `Up Less than a second` because `InvocationResults` is in-memory only. Fix: invoke goroutine writes result to function tags; `ReplayInvocationsFromCloudWatch` rebuilds map on startup. |
| 812 | L | lambda | `docker ps -a` reported `292 years ago` — AWS Lambda's `LastModified` format isn't RFC3339-parseable. Fix: parse AWS-specific format in `cloud_state.go::queryFunctions`, re-emit RFC3339Nano. |
| 813 | M | ecs | `TestECSArithmeticInvalid` flaked because `waitForTaskRunning` only treated exit-code-0 STOPPED as success. Fix: `taskHasContainerExitCode` — any user exit code = start succeeded; only platform failure (no ExitCode) = start failed. |
| 815 | H | ecs | `docker exec <ecs-cid> echo hi` returned no output — `cloudExecStart` passed shell-feature script directly to ECS exec. Fix: wrap script in `sh -c <quoted>`. |
| 816 | M | ecs | `docker diff` produced busybox find help text — `find … -printf` is GNU-only. Fix: use `for t in d f l; do find … -type "$t" \| sed …` (busybox + GNU compatible). |
| 817 | M | ecs | `docker cp` returned `unexpected stat output: "/path\\t…"` — busybox `stat` doesn't interpret `\t` in single-quoted format. Fix: literal tab characters. |
| 818 | H | sim/aws | After BUG-815's `sh -c` wrap, simulator's exec WebSocket left surrounding single quotes intact when handing the script to docker exec. Fix: drop unwrap branch; always wrap (double-wrapping is correct). |
| 819 | H | terraform | Round-9 ECS teardown hung 30+ min on hyperplane-ENI release. Fix: per-cloud `null_resource sockerless_runtime_sweep` clears VpcConfig before delete; ECS module polls for AWS-Lambda ENIs to flip available. |
| 789 / 798 | H | ecs | `docker top` / `docker diff` returned `ps failed (exit -1)` — SSM `Interactive=true` doesn't send exit-code frames for one-shot commands. Fix: wrap every command in `sh -c '<cmd>; printf "__SOCKEXIT:%d:__" $?'`; `extractSSMExitMarker` recovers exit code. |
| 802 | — | (withdrawn) | C5 export 0-byte tar — was a `timeout 60` artefact, not a code bug. Subsumed by BUG-789/798 fix. |
| 638 / 640 / 646 / 648 | H/M | core (registry) | (Retroactive) ECR push only stored manifests, never blobs; `ImagePush` returned synthetic "Pushed"; OCI push fell back to empty-gzip layers; `FetchImageMetadata` synthesised metadata on registry unreachable. All closed by BUG-788 (real layer mirror) + Phase 90 no-fakes audit. |

### Round-8 (PR #118) (BUG-786..800)

| ID | Sev | Area | One-liner |
|----|-----|------|-----------|
| 786 | M | core | `docker rmi <tag>` reported untagged but tag reappeared in `docker images`. Root: `StoreImageWithAliases` puts under multiple keys; partial-untag missed alias entries. Fix: sweep every Store entry with matching ID. |
| 787 | L | docs | `specs/CLOUD_RESOURCE_MAPPING.md` lagged implementation across multiple phases. Fix: per-cloud spelling rule, Phase 91-94 Volume row updated, ECS Container* ops via SSM, Acceptable-gaps section added. |
| 788 | H | core | `docker push <ecr-uri>` failed with `image has no layer data available` — `ImagePull` only stored metadata, not blob bytes. Fix: `FetchLayerBlob` downloads each layer to `Store.LayerContent`; `OCIPush` uses cached compressed bytes verbatim. |
| 790 | M | ecs | `docker stop` returned success before ECS reached STOPPED, breaking `docker rm`. Fix: `waitForTaskStopped` blocks until LastStatus=STOPPED (caller-supplied timeout + 30s grace). |
| 791 | L | core | `docker cp` returned misleading `404 Not Found for API route`. Fix: `handleGetArchive` / `handleHeadArchive` route through `WriteError` like `handlePutArchive`. |
| 792 | L | ecs | `docker commit` error message contained `Phase 98b` reference. Fix: rewritten to describe architectural prerequisite without phase name. |
| 793 | H | lambda + tf | Lambda failed `CreateNetworkInterface` permission when `SOCKERLESS_LAMBDA_SUBNETS` set. Fix: terraform attaches `AWSLambdaVPCAccessExecutionRole`. |
| 794 | H | ecs | Cross-network isolation broken — default SG attached regardless of network membership. Fix: when `SecurityGroupIDs` non-empty, use ONLY those SGs. |
| 795 | M | core | `podman ps --filter name=svc` returned empty even when `svc1` running. Fix: `name` filter uses `strings.Contains` not `==`. |
| 796 | M | ecs (libpod) | `podman pod rm` failed with running-containers error after `pod stop`. Fix: transitively closed by BUG-790's `waitForTaskStopped`. |
| 797 | M | lambda | `public.ecr.aws/...` images failed with `manifest not supported` because Lambda rewrote through ECR pull-through. Fix: short-circuit `public.ecr.aws/` like ECS (BUG-776). |
| 799 | M | ecs | After backend restart, `docker rmi` returned conflict for STOPPED-task images. Fix: `ScanOrphanedResources` skips STOPPED/DEPROVISIONING tasks; only RUNNING/PENDING become "active orphans". |
| 800 | H | core | **Stateless invariant violation.** `core.ResourceRegistry` persisted to `./sockerless-registry.json` by default — operator's CWD became load-bearing. Fix: `Save`/`Load`/`autoSave` collapsed to no-ops; cloud is the source of truth via `sockerless-managed=true` tag scan. |

### Round-7 (PR #117) (BUG-770..785)

| ID | Sev | Area | One-liner |
|----|-----|------|-----------|
| 770 | M | core | `ImageRemove` reported phantom untag/delete for multi-tagged images. Fix: resolve user ref against `RepoTags`; only matching tag returns Untagged. |
| 771 | M | ecs | `ContainerInspect` returned empty Path/Args/Cmd/Entrypoint for cloud-derived containers. Fix: lazy `describeTaskDefinition` cache populates fields from TaskDefinition. |
| 772 | M | ecs | `docker restart` didn't bump RestartCount. Fix: `sockerless-restart-count` tag on RunTask; `taskToContainer` reads it back. |
| 773 | H | ecs | `docker rename` desynced in-memory Store from cloud. Fix: `Server.ContainerRename` calls `TagResource` to overwrite `sockerless-name` tag. |
| 774 | H | ecs | `docker ps -a` returned duplicate rows after `docker restart`. Fix: `queryTasks` dedupes by `sockerless-container-id` tag, prefers RUNNING over STOPPED. |
| 775 | H | ecs | `docker rm` no-op'd because ECS keeps STOPPED visible ~1h. Fix: `ContainerRemove` writes to `ResourceRegistry`; `queryTasks` filters cleaned-up ARNs. |
| 776 | H | ecs | `ContainerCreate` silently fell back to raw image ref on ECR pull-through failure. Fix: `buildContainerDef` returns the resolution error; `public.ecr.aws/` short-circuits. |
| 777 | H | core (auth) | `docker push <ecr-uri>` always returned 401. Fix: `ImageManager.Push` always replaces caller-supplied auth with fresh `Auth.GetToken(registry)`. |
| 778 | M | core (libpod) | `podman inspect` failed with `parsing time ""`. Fix: `normalizeContainerTimes` fills empty stamps with zero-time. |
| 779 | H | core (libpod) | `GET /libpod/containers/json` returned `[]` on stateless backends. Fix: `handleLibpodContainerList` queries `CloudState.ListContainers`. |
| 780 | H | core (libpod) | `podman create --name` → `start` failed `no such container` because libpod specgen JSON is flat. Fix: handler reads specgen fields directly. |
| 781 | L | ecs | `docker kill -s SIGTERM` reported raw container exit code, not Docker's 128+signum. Fix: `sockerless-kill-signal` tag; `mapTaskStatus` overrides ExitCode. |
| 782 | L | ecs | `docker stats` NAME column showed `"--"`. Fix: `buildStatsEntry` uses `ResolveContainerAuto`. |
| 783 | H | ecs | Per-network SG never attached to Fargate ENIs. Fix: `ContainerCreate` calls `cloudNetworkConnect` at create time when initial network is user-defined. |
| 784 | H | ecs | `docker compose up` failed `Some tags contain invalid characters` because Docker labels were raw JSON in ECS tags. Fix: `sockerless-labels-b64` URL-safe base64, chunked when >256 chars. |
| 785 | L | core | `ResourceRegistry.autoSave` logged rename warnings on every op. Fix: `MkdirAll` parent before writing. |

### Pre-Round-7 highlights — Phases 86-105 (BUG-661..769)

Compressed; full per-bug detail in `git log` and PR descriptions for #112 / #113 / #114 / #115. Per-cloud ordering preserved.

| ID range | Theme | One-liner roll-up |
|----------|-------|-------------------|
| 762..769 | Phase 102 / FaaS auth / OCI push | ECS exec via SSM ExecuteCommand for `top`/`stat`/`tar`/`pause`/`unpause`; pidfile convention; OCIPush gets real `Architecture`/`OS`/`Config`/`diff_ids`; auth flow always replaces caller token with fresh `Auth.GetToken`. |
| 749..761 | Phase 96 / 98 / 99 / 100 / 101 / GCF/AZF parity | Reverse-agent for Cloud Run + ACA; pause/unpause via `/tmp/.sockerless-mainpid`; bootstrap teest stdout to container stdout; Lambda invocation tracking; ACA console exec WebSocket; podman pods for docker backend. |
| 744..748 | Phase 94 / 95 / 97 | Lambda EFS volumes; FaaS invocation-lifecycle tracker (`InvocationResult` + tag-based replay); GCP label-value charset compliance via annotations. |
| 738..743 | Phase 91-93 / Lambda runtime | EFS access points (ECS); GCS volumes (Cloud Run); Azure Files (ACA); Lambda short-lived task short-circuit; Lambda overlay-image pre-resolved when configured. |
| 731..737 | Phase 90 no-fakes audit | `VolumeCreate` returns NotImplemented across cloud backends; `ImagePullWithMetadata` propagates registry errors (no synthetic placeholder); `getNamespaceName` returns errors instead of silent ID substitution; `stats` PIDs=0 instead of fabricated 1. |
| 727..730 | CI integration tests | Unit job CI wiring (`SOCKERLESS_INTEGRATION=1`); `noui` build tag; per-backend integration tests; SSM ack format matches AWS Java-style `putUuid`. |
| 720..726 | Round-6 ECS exec + state | SSM AgentMessage parser/ack writer; full SSM permissions in IAM; `EnableExecuteCommand: true`; cloud-derived NetworkState/Pods/Images; Cloud Map post-RUNNING registration with real ENI IP. |
| 712..719 | Phase 86 ECS networking | Idempotent network/namespace create; `DnsSearchDomains` via `/bin/sh -c`-wrapped argv; default port `:3375`; `SOCKERLESS_ECR_DOCKERHUB_CREDENTIAL_ARN` wired with explicit error. |
| 700..711 | Phase 86 cross-cloud sims + DNS | Per-namespace docker network in sims; `pre-register subnet-sim` in EC2 sim; ACA Private DNS Zones via `armprivatedns`; `LinkResource.OnPush` no-op variant; ACR cache-rule CRUD; Cloud Build secret manager. |
| 685..699 | Phase 86 simulator parity | Real Lambda Runtime API; sim workloads via Docker SDK; ServiceV2/JobV2 LatestCreatedExecution; SystemData on ContainerAppJob; `ResolveLocalImage` registry-URI mapping; container-status conditional fixes; `AllowCreated` for `StreamCloudLogs`. |
| 661..684 | Stateless invariants | Auto-agent removed across cloud backends; CloudState replaces Store reads in 16 callsites; `ResolveContainerAuto` migration; `CloudState.ListContainers` for list ops; CloudPodLister merged into PodList. |

Pre-661 entries are summarised in the original git log + per-PR descriptions. Per-bug detail before BUG-661 is too terse to be useful here.

## Cross-cloud sweep notes

When a bug is found, similar code paths in other backends / clouds / simulators are checked too. Notable sweep outcomes:

- BUG-708 ECS-only — Azure uses ACR cache rules with managed-identity auth; GCP uses Artifact Registry pull-through (different mechanism).
- BUG-709 ECS-only — Azure ACA's SDK helper sleeps internally; GCP's SDK handles polling.
- BUG-710 swept all 7 backend mains + CLI + READMEs + example terraform + `tools/http-trace`.
- BUG-711 ECS-only — no other backend sets explicit DNS search domains; GCP/Azure rely on per-network DNS zones with FQDN resolution.
- BUG-712 → found BUG-713 in cloudrun (same non-idempotent-create pattern). Azure ACA's `BeginCreateOrUpdate` is PUT-style and idempotent; Lambda/GCF/AZF have no cloud-side network creation.
- BUG-714 → found BUG-715 (cloudrun) and BUG-716 (aca) with the same placeholder-IP symptom. All three backends seed `ep.IPAddress = "0.0.0.0"` at create time; each needed a different structural fix (ECS got the real Fargate ENI IP; cloudrun + aca moved to Services/Apps with stable FQDNs in Phase 87/88).
- BUG-826 swept all 6 cloud backends — the synthetic `exitCode=0` pattern was identical in `core/backend_impl.go`, `core/backend_impl_pods.go`, `ecs/backend_impl.go`, `aca/backend_impl.go`, `cloudrun/backend_impl.go`, `cloudrun-functions/backend_impl.go`, `azure-functions/backend_impl.go`, `azure-functions/backend_impl_pods.go`.
- BUG-829 swept gcp-common and azure-common — same per-tag silent-continue pattern as BUG-825.
