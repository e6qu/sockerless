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
- **92–94, 94b** — Real per-cloud volume provisioning. GCF/AZF landed earlier; Lambda EFS via `FileSystemConfigs[]` closed BUG-748.
- **95** — FaaS invocation-lifecycle tracker. `core.InvocationResult` + `Store.{Put,Get,Delete}InvocationResult` capture exit codes at the invocation source; CloudState overlays them. Closes BUG-744.
- **96** — Reverse-agent exec for Cloud Run Jobs + ACA Jobs. Shared `core.ReverseAgent{Registry,ExecDriver,StreamDriver,HandleReverseAgentWS}`; CR/ACA mount `/v1/<backend>/reverse`, wire `Drivers.Exec`/`Drivers.Stream`, inject `SOCKERLESS_CALLBACK_URL` + `SOCKERLESS_CONTAINER_ID`. Closes BUG-745.
- **97** — Docker labels on FaaS/GCP backends (charset-safe encoding). `core.AsGCPLabels` / `AsGCPAnnotations`; GCF carries labels as base64-JSON `SOCKERLESS_LABELS` env var. Closes BUG-746.
- **98** — Agent-driven container filesystem + introspection ops. `core.RunContainer{Top,StatPath,GetArchive,PutArchive,Export,Changes}ViaAgent` helpers + shared parsers. Closes BUG-751/752/753.
- **98b** — Agent-driven `docker commit` (opt-in via `SOCKERLESS_ENABLE_COMMIT=1`). `core.CommitContainerViaAgent` runs `find + tar` over the reverse-agent to package a proper diff layer stacked on the source image's rootfs. Closes BUG-750.
- **99** — Agent-driven pause/unpause via SIGSTOP/SIGCONT. Bootstraps write user-process PID to `/tmp/.sockerless-mainpid`; `core.RunContainer{Pause,Unpause}ViaAgent` signals it over the agent WS. Closes BUG-749.
- **100** — Docker backend pod synthesis via the shared `sockerless-pod` label convention. PodList merges Store.Pods with live Docker containers so restarts don't drop pods. Closes BUG-754.
- **101** — Simulator parity for cloud-native exec/attach. Azure sim serves `Microsoft.App/jobs/{job}/executions/{exec}/exec` bridged to real `docker exec`; `core.AttachViaCloudLogs` gives every FaaS backend a read-only log-streamed attach fallback. Closes BUG-760.
- **102** — ECS parity via SSM. `backends/ecs/ssm_capture.go::RunCommandViaSSM` captures stdout/stderr/exit over SSM AgentMessage frames; `ssm_ops.go` wraps it for Export/Top/Changes/StatPath/cp/Pause. Closes BUG-761/762.
- **Audit sweep (PR #115)** — 13 additional bugs filed + fixed (BUG-756 through BUG-769). `ContainerAttach`/`ExecStart` dispatch, `OnPush`/`OCIPush` correctness, base64(JSON) argv, PID-file publishing, heartbeat mutex, overlay-build hard-fail, `ImageHistory` fake removal.

## Pending work

### Live-cloud validation runbooks

- **Phase 87 live-GCP** — parallel to `scripts/phase86/*.sh` for AWS. Needs GCP project + VPC connector. Script the runbook, dispatch via a new workflow, validate `docker run` / `docker exec` / cross-container DNS against Services.
- **Phase 88 live-Azure** — same shape for ACA. Needs Azure subscription + managed environment with VNet integration.
- **Phase 86 Lambda live track** — scripted already, deferred at Phase C closure for session-budget reasons. No architectural blockers.


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
