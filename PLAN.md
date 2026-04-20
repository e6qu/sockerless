# Sockerless — Roadmap

> 86 phases complete (757 tasks). 726 bugs tracked (720 fixed + 3 Phase-89-in-progress + 3 open across Phase 87/88/89). Phase 86 Phase C closed 2026-04-20 with ECS bound live-validated. Phase 89 first checkpoint landed.
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
| 715 | Cloud Run | open — split into Phase 87 (architectural rewrite Jobs → Services with internal ingress + VPC connector). Doesn't block ECS+Lambda runbook. |
| 716 | ACA | open — split into Phase 88 (architectural rewrite Jobs → Apps with internal ingress). Doesn't block ECS+Lambda runbook. |
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

1. **Cloud-resource mapping doc** (`docs/CLOUD_RESOURCE_MAPPING.md`): formal mapping per cloud:
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

## Phase 87 — Cloud Run rewrite (Jobs → Services with internal ingress) — queued

Closes BUG-715. Cloud Run Jobs don't expose addressable per-execution IPs reachable from peer Jobs, so cross-container DNS via Cloud DNS A-records is fundamentally broken on the current backend. Move container execution from `Jobs.RunJob` to `Services.CreateService` with `--ingress=internal-and-cloud-load-balancing` and a VPC connector; per-network managed Service per container hostname; Service's allocated internal IP becomes the Cloud DNS A-record target. Add `cloudrun.UseService` config flag with deprecation path for the Jobs flow. Touches: `backends/cloudrun/{containers,backend_impl,jobspec,backend_impl_network}.go`, `service_discovery_cloud.go`, terraform examples, integration tests.

## Phase 88 — ACA rewrite (Jobs → Apps with internal ingress) — queued

Closes BUG-716. Same shape as Phase 87 for Azure: ACA Jobs aren't peer-addressable; move to ACA Apps with `Ingress.External=false` per container hostname inside the network's environment. App's stable FQDN/IP becomes the Private DNS A-record target. Add `aca.UseApp` config flag. Touches: `backends/aca/{containers,backend_impl,backend_impl_network,service_discovery_cloud}.go`, terraform examples, integration tests.

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
