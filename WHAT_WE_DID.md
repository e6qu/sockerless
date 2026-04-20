# Sockerless — What We Built

Docker-compatible REST API that runs containers on cloud backends (ECS, Lambda, Cloud Run, GCF, ACA, AZF) or local Docker. 7 backends, 3 cloud simulators, validated against SDKs/CLIs/Terraform.

86 phases, 757 tasks, 726 bugs tracked (720 fixed + 3 Phase-89-in-progress + 3 open). See [STATUS.md](STATUS.md), [BUGS.md](BUGS.md), [specs/](specs/), [docs/CLOUD_RESOURCE_MAPPING.md](docs/CLOUD_RESOURCE_MAPPING.md).

## Phase 86 — Simulator parity + Lambda agent-as-handler

Closes the Phase 86 plan: every cloud-API slice sockerless depends on is now a first-class cloud-slice in its per-cloud simulator, validated with SDK + CLI + terraform tests (or an explicit exemption). Lambda's agent-as-handler pattern for `docker exec` / `attach` is fully wired up: bootstrap loop + overlay image build + reverse-agent WebSocket server.

Branch: `phase86-complete-runner-support` → PR #112 (merged 2026-04-20 as commit `7f054e0`).

Phase C — live-AWS session 2 — is in progress on `post-phase86-continuation` off `origin/main`. Session 1's two blocker bugs (BUG-692 docker-run hang, BUG-P86-A2 raw ECS image ref) are fixed; session 2 reruns the full runbook (0-infra-up → 6-teardown) + the e2e runner matrix. Plan at `~/.claude/plans/purring-sprouting-dusk.md`.

### Live-AWS session 2 — bugs found + fixed in-flight (2026-04-20)

Session 2 (cluster `sockerless-live`, account 729079515331, eu-west-1) ran Phase 1 ECS infra-up green (34 resources via terragrunt apply, ~2min) then surfaced five product bugs while attempting Phase 2 smoke. Each was filed in BUGS.md and fixed before retrying the relevant smoke step:

- **BUG-708** ECR pull-through cache for docker-hub now requires a Secrets Manager credential ARN; backend was logging WRN per-create + falling back to direct upstream pull (which actually works for public images). Fixed: per-prefix "skip cache" memo + WRN→INF demotion noting the env var to enable proper auth. Full credential plumbing deferred.
- **BUG-709** `waitForOperation` polled Cloud Map's `GetOperation` 60× back-to-back with no sleep — burned 60 API calls in <10s while real Cloud Map needs 30-60s. Fixed: `time.Sleep(2*time.Second)` between polls (60 × 2s = 120s headroom) + DBG log per pending tick.
- **BUG-710** Sockerless CLI default `--addr` was `:2375`, colliding with Docker daemon's well-known port (and Podman's). Fixed: change all defaults — CLI server start, all 7 backend `cmd/*/main.go`, READMEs, example terraform outputs, `tools/http-trace` — to `:3375`. Test fixtures left at their existing arbitrary values.
- **BUG-711** P86-003 set `ContainerDefinition.DnsSearchDomains` so awsvpc tasks could resolve bare short names against their network's Cloud Map namespace. ECS `RegisterTaskDefinition` rejects this for awsvpc unconditionally. Minimum fix: drop the field; cross-container DNS now FQDN-only (`svc.skls-<net>.local`). Long-term mechanism (DHCP options vs. resolv.conf injection vs. Service Connect) deferred.
- **BUG-712** `cloudNetworkCreate` non-idempotent: retry after partial failure / leftover state crashed at `CreateSecurityGroup` (`InvalidGroup.Duplicate`) and `CreatePrivateDnsNamespace` (`ConflictingDomainExists`). Fixed: catch `InvalidGroup.Duplicate` and look up existing SG by name+VPC; tolerate `InvalidPermission.Duplicate` on self-ingress; new `findNamespaceByName` helper called before `CreatePrivateDnsNamespace` to reuse existing namespace.
- **BUG-713** Cross-cloud sweep of BUG-712 found the same idempotency gap in `backends/cloudrun/network_cloud.go::cloudNetworkCreate` — GCP `ManagedZones.Create` returns 409 on retry, leaving the network unusable. Fixed: catch `googleapi.Error{Code: 409}` and fall back to `ManagedZones.Get` to reuse the existing zone. Azure ACA verified naturally idempotent (PUT semantics via `BeginCreateOrUpdate`); Lambda + GCF + AZF verified to have no cloud-side network creation.
- **BUG-714** ECS Cloud Map registered each container with `ep.IPAddress` from the local container state — that field is seeded as the placeholder `"0.0.0.0"` and never updated with the real Fargate ENI IP. Effect: Cloud Map A-records resolved to `0.0.0.0`, breaking cross-container DNS even by FQDN. Fixed in `backends/ecs/backend_impl.go::startSingleContainerTask`: move the registration loop AFTER `waitForTaskRunning(...)` returns the task's `ip:9111` address (already extracted by `eni.go::extractENIIP`); strip the port and pass that real IP into `cloudServiceRegister`.
- **BUG-715, BUG-716** Cross-cloud sweep of BUG-714 found same placeholder-IP-into-DNS pattern in `backends/cloudrun/service_discovery_cloud.go::cloudServiceRegister` and `backends/aca/service_discovery_cloud.go::cloudServiceRegister`. Cloud Run Jobs and ACA Jobs don't have addressable per-execution IPs reachable from other Jobs the way Fargate ENIs are. Architectural rewrites needed: Cloud Run → Services with internal ingress + VPC connector (Phase 87); ACA → Apps with internal ingress (Phase 88). Tracked as open bugs in their respective future phases.
- **BUG-717** `docker exec <ecs-container>` returned `unrecognized stream: 69` because SSM Session Manager binary frames were piped through Docker mux without being decoded. Fixed in new `backends/ecs/ssm_proto.go` (full AgentMessage parser + ack writer + input wrapper, 120-byte header + payload-type routing) and `backends/ecs/exec_cloud.go::ssmDecoder` (frame-by-frame `io.ReadFull` reads, ack emission, `channel_closed` / `exit_code` termination). 7 unit tests. Live-verified after BUG-719/720/721/722 fixes.
- **BUG-719** ECS Exec also requires `RunTask.EnableExecuteCommand: true` set at task launch — there's no way to enable it after the fact. Fixed in `backends/ecs/containers.go::startTask`.
- **BUG-720** Task IAM role lacked `ssmmessages:CreateControlChannel/CreateDataChannel/OpenControlChannel/OpenDataChannel` permissions required for the in-task SSM agent to dial back to Session Manager. Fixed in `terraform/modules/ecs/main.tf::data.aws_iam_policy_document.task` with new `ECSExecSSMMessages` statement.
- **BUG-721** SSM agent retransmits each `output_stream_data` frame until it sees an ack it accepts; sockerless's ack format isn't (yet) recognized despite matching session-manager-plugin source layout. Pragmatic dedupe by `MessageID` in `ssmDecoder` so docker sees correct output (without it, `echo single` showed up 10× in the docker output). Long-term work to nail down the agent's exact ack-validation rules tracked separately.
- **BUG-722** After backend restart, `s.ECS.Get(containerID)` returned empty `ECSState`, breaking `cloudExecStart` with `no ECS task associated with container <id>` (first byte 'n'=110 → docker CLI: `unrecognized stream: 110`). Fixed by new `ecsCloudState.resolveTaskARN(ctx, containerID)` (ListTasks + DescribeTasks + tags filter); `cloudExecStart` calls it as fallback when in-memory state is empty. Phase 89 will replace this lazy recovery with consistent cloud-derived lookups across all callsites.
- **BUG-718** Cross-cloud sweep of BUG-708 found same docker-hub credential issue + a separate silent `pushToECR` fallback in `backends/lambda/image_resolve.go`. Both fixed: same credential-ARN wiring, `pushToECR` removed (only worked for pre-loaded local-store images, swapped image source without operator awareness).

### Phase 2 ECS smoke — final live results (2026-04-20 session 2)

- 2.1 `docker run --rm alpine echo` → **PASS** (~33s, Fargate cold start)
- 2.2 `docker run -d` + `docker logs` → **PASS** (tick-1/2/3 streamed from CloudWatch)
- 2.3 cross-container DNS via FQDN (`curl http://svc.skls-net.local:8080`) → **PASS** (BUG-709/711/712/714 all live-verified)
- 2.3 short-name (`curl http://svc:8080`) → **PASS** (BUG-711 entrypoint shim live-verified — resolv.conf has `nameserver 10.99.0.2` + `search skls-skls4.local`)
- 2.4 `docker exec svc echo single` → **PASS** (single line output; BUG-717/719/720/721/722 all live-verified)

Phase 86 Phase C closes here. Lambda track + e2e tests deferred to future session — no architectural blockers, ECS bound was the priority for live validation. AWS infra fully torn down post-validation, zero residue (state buckets retained as cheap reusable infra).

### Simulator parity (AWS + GCP + Azure)

- **A.5** — Pre-commit testing contract enforced in `.pre-commit-config.yaml` + `AGENTS.md`: every `r.Register("X", ...)` addition needs a matching SDK + CLI + terraform-tests entry, or an explicit opt-out in `simulators/<cloud>/tests-exempt.txt`.
- **BUG-696** — AWS ECR pull-through cache slice (`CreatePullThroughCacheRule` / `DescribePullThroughCacheRules` / `DeletePullThroughCacheRule` + URI rewriting).
- **BUG-697** — `Store.Images` persistence across backend restart (six cloud backends; default path `~/.sockerless/state/images.json`).
- **BUG-700** — Cloud-side network-create failures surface as response `Warning` on ECS, Cloud Run, and ACA (was silently dropping DNS + security-group errors).
- **BUG-701** — Cross-task DNS via real Docker networks: AWS Cloud Map namespaces, GCP Cloud DNS private zones, Azure ACA environments. Shared-library helpers `EnsureDockerNetwork` / `ConnectContainerToNetwork` in each cloud's `shared/container.go`.
- **BUG-702** — Azure Private DNS Zones backend SDK wire (`armprivatedns`). In-memory `serviceRegistry` removed entirely.
- **BUG-703** — Azure NSG backend SDK wire (`armnetwork/v7`) + simulator `securityRules` sub-resource CRUD consistent with the NSG's `Properties.SecurityRules` array.
- **BUG-704** — GCP Cloud Build slice with real `docker build` execution, LRO polling, streaming logs.
- **BUG-705** — AWS Lambda Runtime API slice: per-invocation HTTP sidecar on `127.0.0.1:<port>` handling `/next`, `/response`, `/error`, `/runtime/init/error`. Container-to-host via `host.docker.internal`.
- **BUG-706** — Azure ACR Cache Rules slice (cacheRule CRUD + pull-through URI rewriting via `ResolveAzureImageURIWithCache`).
- **BUG-707** — GCP Cloud Build Secret Manager integration (availableSecrets → runtime env var resolution via the new `simulators/gcp/secretmanager.go` slice).
- **GCP Cloud Run v1 services** — Knative-style CRUD for parity completeness.

See `docs/SIMULATOR_PARITY_{AWS,GCP,AZURE}.md` for the complete slice matrix. Zero ✖ rows on the runner path.

### Phase D — Lambda agent-as-handler

- **D.1** `agent/cmd/sockerless-lambda-bootstrap/main.go` — real Runtime-API polling loop. Parses `Lambda-Runtime-*` headers, spawns user entrypoint + CMD with invocation payload on stdin, posts `/response` (or `/error` envelope). Reverse-agent WebSocket dialed once at init with 20s heartbeat.
- **D.2** `backends/lambda/image_inject.go` — `BuildAndPushOverlayImage` renders the overlay Dockerfile, stages agent + bootstrap binaries, runs `docker build` + `docker push` to the destination ECR URI. `ContainerCreate` calls it when `CallbackURL` is set.
- **D.3** `backends/lambda/reverse_agent_server.go` — WebSocket upgrade at `/v1/lambda/reverse?session_id=...` mounted on the BaseServer mux. `reverseAgentRegistry` handles register/resolve/drop with reconnect-same-session-id resume semantics.
- **D.4** `lambdaExecDriver` + `lambdaStreamDriver` route `docker exec` / `docker attach` through the reverse-agent session. Real end-to-end test at `backends/lambda/agent_e2e_integration_test.go` (gated on `SOCKERLESS_INTEGRATION=1`): builds the real bootstrap + bakes into a test image; runs real docker + AWS simulator + Lambda backend; `docker run` → Lambda invoke → sim spawns handler → bootstrap dials back via `host.docker.internal` → `docker exec` resolves via `lambdaExecDriver` → bootstrap spawns subprocess → stdout returns. Passes in ~1.5s. Post-stop path verified too.

### CI codification

- `scripts/phase86/0-infra-up.sh` through `6-teardown.sh` — idempotent shell scripts for each runbook.
- `.github/workflows/phase86-aws-live.yml` — `workflow_dispatch`-only with sensitive tokens as inputs. Teardown runs under `if: always()` so a failed earlier job still releases scratch AWS resources.

### Live-AWS (Phase E)

Awaiting AWS credentials. The `runner-capability-matrix.md` live columns stay pending-live until the workflow is dispatched successfully.

## Phase 89 — Stateless backend audit + cloud-resource mapping (in progress)

Per the user's stateless-backend directive ("backends should be stateless; state derived from cloud actuals; ECS tasks → containers/pods, sockerless-tagged SG + Cloud Map namespace → docker network"). First checkpoint landed in this branch:

- **`docs/CLOUD_RESOURCE_MAPPING.md`** — canonical cross-backend mapping (docker container/pod/network/image/exec → cloud resource per backend), state-derivation rules, recovery contract, list of currently-violating in-memory state.
- **BUG-723 step 1** — Removed `Store.Images` disk persistence: `Store.PersistImages` / `Store.RestoreImages` / `Store.ImageStatePath` / `DefaultImageStatePath` deleted; `NewBaseServer` no longer auto-restores from `~/.sockerless/state/images.json`; `StoreImageWithAliases` no longer auto-persists. `Store.Images` is now a pure in-process cache. Per-backend cloud-derived `docker images` is the next step.
- **BUG-725 ECS** — New `Server.resolveTaskState(ctx, containerID)` in `backends/ecs/cloud_state.go` wraps cache + cloud-derived fallback (calls `resolveTaskARN`, writes through to cache). Refactored callsites: `ContainerStop`, `ContainerKill`, `ContainerRemove` (with `DescribeTasks` to recover `TaskDefinitionArn` for deregister too), `cloudExecStart`. After restart, `docker stop`/`kill`/`rm`/`exec` work without rehydrating in-memory state first.
- **BUG-725 Lambda** — Mirror: `Server.resolveLambdaState(ctx, containerID)` + `lambdaCloudState.resolveFunctionARN(ctx, containerID)`. Refactored `ContainerStop`, `ContainerKill`.
- **BUG-726 ECS** — New `Server.resolveNetworkState(ctx, networkID)` derives `SecurityGroupID` via `DescribeSecurityGroups Filters=[tag:sockerless:network-id=<id>]` and `NamespaceID` via `ListNamespaces` filter by `tag:sockerless:network-id=<id>`. `cloudNamespaceCreate` now tags the namespace at create time (`sockerless:network-id`, `sockerless:network`, `sockerless-managed`).

Still TBD:

- BUG-723 step 2: per-backend cloud-derived `docker images` (currently the in-memory cache repopulates lazily on each `docker pull`).
- BUG-724: `PodList` from cloud actuals (multi-container task / app grouped by `sockerless-pod` tag) — currently still uses `Store.Pods` local registry.
- BUG-725 cloudrun + aca: same `resolve*State` pattern needed.
- BUG-726 cloudrun + aca: same `resolveNetworkState` pattern needed.
- Restart-resilience integration tests per backend.
