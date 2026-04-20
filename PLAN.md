# Sockerless — Roadmap

> 86 phases complete (757 tasks). 717 bugs tracked (715 fixed, 2 open: BUG-715 cloudrun rewrite, BUG-716 aca rewrite — both architectural).
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

### Phase 86 Phase C — open bugs (must all be fully fixed before live runbook resumes)

Per the user's directive (no defers, no fakes): every open bug below blocks the live runbook. Fix each in the same branch; no "minimum fix + defer." Cross-cloud sweep is mandatory.

| Bug | Backend(s) | Fix required (no minimums) |
|---|---|---|
| 708 | ECS | Wire `SOCKERLESS_ECR_DOCKERHUB_CREDENTIAL_ARN` env var into `ensurePullThroughCache` as `CredentialArn` on `CreatePullThroughCacheRule`. When set, the rule registers correctly; when unset, document the limitation but DO NOT silently fall back — return a clear error so the operator knows credentials are needed. |
| 709 | ECS | Done (`waitForOperation` sleeps + DBG logs). Add unit test `TestWaitForOperation_PollsWithSleep` using a fake `ServiceDiscovery` client that returns PENDING three times then SUCCESS. |
| 710 | All 7 backends + CLI | Done (defaults all to `:3375`). Add CI assertion via grep that no source/markdown file has hardcoded `:2375` or `:9100` outside fixtures. |
| 711 | ECS | Implement real cross-container short-name DNS via VPC DHCP option set: backend creates a per-cluster DHCP option set with the Cloud Map namespace as the search domain, associates it with the VPC on first network create, restores VPC's prior options on last network delete. OR: ship a per-task entrypoint shim that prepends `search <namespace>` to `/etc/resolv.conf` (less invasive). Pick option B — DHCP modification is too disruptive to a shared VPC. Implement entrypoint-prepend logic in `containerDef.Command` build path. |
| 712 | ECS | Done (`cloudNetworkCreate` idempotent for SG + namespace). Add unit tests for both reuse paths against a fake EC2 + ServiceDiscovery client. |
| 713 | Cloud Run | Done (`cloudNetworkCreate` reuses zone on 409). Add unit test using a fake DNS client that returns 409 then succeeds on Get. |
| 714 | ECS | Done (Cloud Map register uses ENI IP after task RUNNING). Add unit test `TestCloudServiceRegister_UsesENIIPNotPlaceholder` with a fake task lifecycle. |
| 715 | Cloud Run | Implement real per-execution IP discovery: poll Cloud Run Job execution `Conditions[?Type=='ContainerReady']` and pull the execution's network IP via the new GCP Compute API path; or move from Jobs to Cloud Run Services with `--ingress=internal` + a VPC connector and use the Service's allocated internal IP. Pick the Service path: it gives stable internal IPs reachable from peer Services in the same VPC. Add `cloudrun.UseService` config flag and a per-network managed Service per container hostname. Update `cloudServiceRegister` to consume the real IP. |
| 716 | ACA | Implement real per-execution IP discovery: ACA Jobs don't have addressable IPs, so move to ACA Apps with `internal` ingress for containers attached to user networks. Backend creates an Container App per hostname inside the network's environment, with `Ingress.External=false`, and registers the App's stable FQDN/IP in the network's Private DNS zone. Add `aca.UseApp` config flag analogous to GCP. Update `cloudServiceRegister` to consume the real IP. |
| 717 | ECS | Implement the SSM Session Manager binary protocol decoder in `backends/ecs/exec_cloud.go`: parse `AgentMessage` (header: HL `int32`, MessageType `string[32]`, SchemaVersion `int32`, CreatedDate `int64`, SequenceNumber `int64`, Flags `int64`, MessageId `[16]byte`, PayloadDigest `[32]byte`, PayloadType `int32`, PayloadLength `int32`); route by `MessageType` ∈ {`output_stream_data`, `acknowledge`, `channel_closed`, ...}; decode payload sub-protocol; extract real stdout/stderr; THEN apply Docker mux header. Reference: AWS `session-manager-plugin` Go source. Includes acknowledge replies + sequence number tracking. |

### Phase 86 Phase C — live runbook (resumes after every bug above is `fixed`)

Live AWS state right now: ECS infra up, Lambda infra terragrunt plan reviewed (6 resources to add). All in-flight backend processes survive across bug-fix iterations.

| Step | Status | Notes |
|---|---|---|
| 0 Preflight | done | Scripts fixed, state buckets bootstrapped, binaries built, creds verified. |
| 1 ECS infra up | done | 34 resources in eu-west-1, ~2min apply. Outputs at `/tmp/ecs-out.json`. |
| 2 ECS smoke | partial-pass + 6 bugs surfaced | 2.1 + 2.2 + 2.3-FQDN PASS; 2.3-shortname blocked on BUG-711 full fix; 2.4 blocked on BUG-717 full fix. Re-run after bug fixes. |
| 3 Lambda infra up | ready | Plan reviewed: 6 resources in us-east-1 (IAM role + 2 policies + log group + ECR repo + lifecycle). Apply queued. |
| 4 Lambda baseline (Runbook 2) | pending | docker run, logs, kill clamp. |
| 5 Lambda agent-as-handler (Runbook 3) | pending | Requires public WebSocket (ngrok / cloudflared) — pause-point D. |
| 6 E2E live tests | pending | github-runner + gitlab-runner × ecs + lambda. Smoke first, widen if budget allows. |
| 7 Teardown | pending | Lambda first, then ECS. Hard requirement before session ends. |
| 8 Final state save | pending | _tasks/P86-AWS-manual-runbook-session2.md, runner-capability-matrix live columns, PLAN/STATUS/WHAT_WE_DID/DO_NEXT updated, branch pushed for PR. |

---

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
