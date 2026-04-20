# Sockerless — Roadmap

> 86 phases complete (757 tasks). 726 bugs tracked (725 fixed-in-code + 1 partially fixed — BUG-724 Phase 89 non-ECS pods). Phase 86 Phase C closed 2026-04-20 with ECS live-validated. Phase 87 (Cloud Run Services) and Phase 88 (ACA Apps) landed 2026-04-21 in code behind UseService/UseApp flags; live-cloud validation pending. Phase 89 near-complete (state derivation + OCI image listing + resolve\*State helpers across all cloud backends).
>
> **Goal:** Replace Docker Engine with Sockerless for any Docker API client — `docker run`, `docker compose`, TestContainers, CI runners — backed by real cloud infrastructure (AWS, GCP, Azure).

## Guiding Principles

1. **Docker API fidelity** — match Docker's REST API exactly
2. **Real execution** — simulators and backends actually run commands
3. **External validation** — proven by unmodified external test suites
4. **No new frontend abstractions** — Docker REST API is the only interface
5. **Driver-first handlers** — all handler code through driver interfaces
6. **LLM-editable files** — source files under 400 lines
7. **GitHub API fidelity** — bleephub works with unmodified `gh` CLI
8. **State persistence** — every task ends with state save
9. **No fakes / no fallbacks / no defers** — every functional gap is a real bug; every bug gets a real fix in the same session it surfaces; cross-cloud sweep on every find

---

## Phase 86 — Complete Runner Support (in progress, Phase C session 2)

Branch `post-phase86-continuation`. Live AWS account `729079515331` (eu-west-1 + us-east-1). ECS infra is up; Lambda infra plan reviewed and ready.

### Phase 86 Phase C — bug status

| Bug | Backend(s) | Status |
|---|---|---|
| 708 | ECS | fixed — `SOCKERLESS_ECR_DOCKERHUB_CREDENTIAL_ARN` wired through `CredentialArn`; explicit error when unset (no fallback). Test `TestDockerHubCredentialARN_ReadsEnv`. |
| 709 | ECS | fixed — `pollOperation` extracted with sleep + injectable poller. 4 unit tests. |
| 710 | All 7 backends + CLI | fixed — defaults `:3375` everywhere. `scripts/check-port-defaults.sh` + pre-commit hook. |
| 711 | ECS | fixed — `/bin/sh -c` entrypoint shim rewrites `/etc/resolv.conf` (preserve VPC nameservers, add namespaces as search line) then `exec`s original argv via `shellQuoteArgs`. Test `TestShellQuoteArgs`. Live verification queued. |
| 712 | ECS | fixed — `cloudNetworkCreate` idempotent (catch `InvalidGroup.Duplicate` + reuse SG by name+VPC; `findNamespaceByName` reuses existing namespace before `CreatePrivateDnsNamespace`). Live-verified during the find. |
| 713 | Cloud Run | fixed — `ManagedZones.Create` 409 → `Get` for reuse. |
| 714 | ECS | fixed — register loop moved AFTER `waitForTaskRunning`; uses real ENI IP via `extractENIIP`. Live-verified (FQDN resolution returned `hi`). |
| 715 | Cloud Run | fixed-in-code 2026-04-21 (Phase 87). Services path behind `SOCKERLESS_GCR_USE_SERVICE=1` + `SOCKERLESS_GCR_VPC_CONNECTOR` writes CNAMEs to `Service.Uri`; Jobs path retained as default. Live-GCP validation pending. |
| 716 | ACA | fixed-in-code 2026-04-21 (Phase 88). Apps path behind `SOCKERLESS_ACA_USE_APP=1` + `SOCKERLESS_ACA_ENVIRONMENT` writes Private DNS CNAMEs to `ContainerApp.LatestRevisionFqdn`; Jobs path retained as default. Live-Azure validation pending. |
| 717 | ECS | fixed — full SSM AgentMessage decoder in `ssm_proto.go` (parse, ack writer, input wrapping). `ssmDecoder` in `exec_cloud.go` reads frame-by-frame with `io.ReadFull`, sends acks, terminates on `channel_closed` / `exit_code`. 7 unit tests. Live verification queued. |
| 718 | Lambda | fixed — cross-cloud sibling of 708 found in lambda's `image_resolve.go`; same credential-ARN wiring + removed silent `pushToECR` fallback (only worked for pre-loaded local-store images, swapped image source without operator awareness). |

### Phase 86 Phase C — live runbook (CLOSED 2026-04-20)

Phase C closes with ECS bound fully validated. Lambda track deferred to a later session — no architectural blockers, just session-time budget.

| Step | Status | Notes |
|---|---|---|
| 0 Preflight | done | Scripts fixed, state buckets bootstrapped, binaries built, creds verified. |
| 1 ECS infra up | done | 34 resources in eu-west-1, ~2min apply. |
| 2.1 `docker run --rm` | **PASS** | Fargate cold start ~33s. |
| 2.2 `docker run -d` + `docker logs` | **PASS** | tick-1/2/3 streamed from CloudWatch. |
| 2.3 cross-container DNS — FQDN (`svc.skls-net.local`) | **PASS** | Cloud Map A-record with real ENI IP after BUG-714 fix. |
| 2.3 cross-container DNS — short name (`svc:8080`) | **PASS** | `/bin/sh -c` entrypoint shim rewrites `/etc/resolv.conf` per BUG-711 fix. |
| 2.4 `docker exec svc echo ...` | **PASS** | SSM Session Manager binary protocol decoded per BUG-717; `EnableExecuteCommand: true` per BUG-719; `ssmmessages:*` IAM per BUG-720; CloudState lazy recovery for TaskARN per BUG-722; ack-retransmit dedupe per BUG-721. |
| 3-5 Lambda track | deferred | Lambda infra not yet provisioned this session. Phase 86 closes with ECS bound proven; Lambda live track is its own future session. |
| 6 E2E live tests | deferred | github-runner + gitlab-runner × ecs + lambda. Same future session as Lambda track. |
| 7 Teardown | done | Cleaned up ECS infra + sockerless-created SGs + Cloud Map namespaces + cluster. Zero residue (state buckets + DDB lock table retained as cheap reusable infra). |
| 8 Final state save | done | This commit. Branch `post-phase86-continuation` for PR. |

Phase C result: **ECS backend live-validated end-to-end.** Of 15 bugs surfaced, 13 fully fixed in-branch (708, 709, 710, 711, 712, 713, 714, 717, 718, 719, 720, 721, 722); 2 split into dedicated future phases (Phase 87 cloudrun rewrite for BUG-715, Phase 88 ACA rewrite for BUG-716). 4 stateless-audit findings (BUG-723/724/725/726) tracked in Phase 89.

---

## Phase 89 — Stateless backend audit + cloud-resource mapping — queued

Closes BUG-723/724/725/726. The current backends keep in-memory state (NetworkState, ECS map, etc.) and persist Store.Images to disk. Per the stateless directive, every backend must derive state from cloud actuals only — the executables of its configured environment.

Concrete deliverables:

1. **Cloud-resource mapping doc** (`specs/CLOUD_RESOURCE_MAPPING.md`): formal mapping per cloud:
   - ECS task → docker container; ECS task with multi-container task-def → podman pod
   - Sockerless-tagged security group + Cloud Map namespace → docker network
   - ECR repository → docker image (queried via DescribeImages)
   - Lambda function → docker container (single-container "pod")
   - Cloud Run Service / Job → docker container (post-Phase-87 it's Services)
   - ACA App / Job → docker container (post-Phase-88 it's Apps)
   - GCF function → docker container
   - Azure Function → docker container
2. **State derivation refactor** in each backend: replace in-memory state stores with on-demand cloud queries (ListTasks + DescribeTasks + tags filter; ListServices + Get; equivalents per cloud). Caching allowed but must be invalidatable.
3. **Remove Store.Images persistence**: query the cloud registry instead.
4. **Backend recovery**: must work after restart with no on-disk or in-memory state.

## Phase 87 — Cloud Run rewrite (Jobs → Services with internal ingress) — done (code), pending live validation

Closes BUG-715 at the code level. The Cloud Run backend now has two parallel execution paths selected by `SOCKERLESS_GCR_USE_SERVICE`; Jobs (default) untouched, Services gated on `SOCKERLESS_GCR_VPC_CONNECTOR` also being set.

| Slice | File(s) | Summary |
|---|---|---|
| 87-01 | `backends/cloudrun/servicespec.go` | `buildServiceName` / `buildServiceSpec` emit `*runpb.Service` with internal ingress + VPC connector egress, MinInstanceCount=MaxInstanceCount=1. |
| 87-02 | `backends/cloudrun/cloud_state_services.go`, `store.go`, `gcp.go` | `CloudRunState.ServiceName`, `GCPClients.Services`, `resolveServiceName`/`resolveServiceCloudRunState`, `queryServices`/`serviceToContainer`, `serviceContainerState` (TerminalCondition → running/exited/created). `ListContainers` now also merges Services when UseService. |
| 87-03 | `backends/cloudrun/start_service.go`, `backend_impl.go` | ContainerStart single/multi-container branches. `startSingleContainerService` + `startMultiContainerServiceTyped` use CreateService + LRO wait. `deleteService` helper. |
| 87-04 | `backends/cloudrun/backend_impl.go` | Stop/Kill/Remove delete the Service (no Cancel equivalent for Services). Cache-cleared so next Start re-creates. |
| 87-05 | `backends/cloudrun/{config,backend_impl,backend_impl_network,service_discovery_cloud}.go` | Validate gate opened (requires VPCConnector). Logs filter switches to `cloud_run_revision` + `service_name`. `cloudServiceRegisterCNAME`/`DeregisterCNAME` write CNAMEs to `Service.Uri` host (no per-instance IP needed; peers reach via VPC connector). |

Unit tests added: service spec shape, service URI parsing, ServiceState cache, serviceToContainer mapping, running/failed/pending state transitions, Validate gate with/without VPC connector. Live validation deferred until GCP credentials + VPC connector are set up in a dedicated runbook (Phase 87 live track, parallel to Phase 86 Phase C for ECS).

Remaining for full closure: live-GCP smoke runbook; integration-test path that spins up a simulator with Services support if we decide to add it to `simulators/gcp/`.

## Phase 88 — ACA rewrite (Jobs → Apps with internal ingress) — done (code), pending live validation

Closes BUG-716 at the code level. The ACA backend now has two parallel execution paths selected by `SOCKERLESS_ACA_USE_APP`; Jobs (default) untouched, Apps gated on `SOCKERLESS_ACA_ENVIRONMENT` also being set.

| Slice | File(s) | Summary |
|---|---|---|
| 88-01 | `backends/aca/appspec.go` | `buildAppName` / `buildAppSpec` emit `armappcontainers.ContainerApp` with `Ingress.External=false`, `ActiveRevisionsMode=Single`, `Scale.{Min,Max}Replicas=1`. |
| 88-02 | `backends/aca/cloud_state_apps.go`, `store.go`, `azure.go` | `ACAState.AppName`, `AzureClients.ContainerApps`, `resolveAppName`/`resolveAppACAState`, `queryApps`/`appToContainer`, `appContainerState` (ProvisioningState → running/exited/created). `ListContainers` also merges Apps when UseApp. |
| 88-03/04 | `backends/aca/start_app.go`, `backend_impl.go` | ContainerStart single/multi-container branches. `startSingleContainerApp` + `startMultiContainerAppTyped` use BeginCreateOrUpdate + PollUntilDone. `deleteApp` helper. Stop/Kill/Remove delete the ContainerApp on the UseApp path. |
| 88-05 | `backends/aca/{config,backend_impl,backend_impl_network,service_discovery_cloud}.go` | Validate gate opened (UseApp requires Environment). Logs filter switches to `ContainerAppName_s` in the same `ContainerAppConsoleLogs_CL` table. `cloudServiceRegisterCNAME` / `DeregisterCNAME` write Private DNS CNAMEs pointing at `ContainerApp.LatestRevisionFqdn`. |

Unit tests added: app spec shape, AppACAState cache, appToContainer mapping, running/failed/in-progress state transitions, Validate gate with/without Environment. Live-Azure validation pending.

## Phase 68 — Multi-Tenant Backend Pools (queued)

Named pools of backends with scheduling and resource limits. Resumes after Phase 86 closes.

| Task | Status | Description |
|---|---|---|
| P68-001 | done | Pool configuration (JSON config) |
| P68-002 | pending | Pool registry (in-memory, each with own BaseServer + Store) |
| P68-003 | pending | Request router (route by label or default pool) |
| P68-004 | pending | Concurrency limiter (per-pool semaphore, 429 on overflow) |
| P68-005 | pending | Pool lifecycle (create/destroy at runtime via management API) |
| P68-006 | pending | Pool metrics (per-pool stats on `/internal/metrics`) |
| P68-007 | pending | Round-robin scheduling (multi-backend pools) |
| P68-008 | pending | Resource limits (max containers, max memory per pool) |
| P68-009 | pending | Unit + integration tests |
| P68-010 | pending | Save final state |

---

## Phase 78 — UI Polish (queued)

Dark mode, design tokens, error handling UX, container detail modal, auto-refresh, performance audit, accessibility, E2E smoke, documentation.

---

## Future Ideas

- GraphQL subscriptions for real-time event streaming
- Full GitHub App permission scoping
- Webhook delivery UI
- Cost controls (per-pool spending limits, auto-shutdown)
