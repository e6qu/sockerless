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

Detail in [WHAT_WE_DID.md](WHAT_WE_DID.md); commit + BUG refs in [BUGS.md](BUGS.md).

| Phase(s) | Summary | Closes |
|---|---|---|
| 86 | Simulator parity (AWS + GCP + Azure) + Lambda agent-as-handler + live-AWS ECS validation | BUG-692–722 |
| 87 / 88 | Cloud Run Services + ACA Apps paths behind `UseService` / `UseApp` flags | BUG-715, 716 |
| 89 | Stateless-backend audit — cloud resource mapping, `resolve*State`, cloud-derived `ListImages` / `ListPods` | BUG-723–726 |
| 90 | No-fakes/no-fallbacks audit (11 bugs filed; 8 fixed in-sweep, 3 scoped as dedicated phases) | BUG-729–737 |
| 91 / 92 / 93 / 94 / 94b | Real per-cloud volume provisioning (EFS / GCS / Azure Files) across all 7 backends | BUG-735, 736, 748 |
| 95 | FaaS invocation-lifecycle tracker — `core.InvocationResult` + exit-code capture at the invocation source | BUG-744 |
| 96 | Reverse-agent exec for CR/ACA Jobs — shared `core.ReverseAgent{Registry,ExecDriver,StreamDriver}` | BUG-745 |
| 97 | Charset-safe GCP labels via annotations / base64-JSON env var | BUG-746 |
| 98 / 98b | Agent-driven `docker cp / export / stat / top / diff / commit` via the reverse-agent | BUG-750, 751, 752, 753 |
| 99 | Agent-driven `docker pause / unpause` via SIGSTOP/SIGCONT over reverse-agent | BUG-749 |
| 100 | Docker backend pod synthesis via the shared `sockerless-pod` label | BUG-754 |
| 101 | Sim parity for cloud-native exec/attach + read-only log-streamed attach fallback for FaaS | BUG-760 |
| 102 | ECS parity for filesystem-ops + pause/unpause via SSM ExecuteCommand | BUG-761, 762 |
| — | Audit sweep (PR #115 follow-up) — dispatch policy, OCI push correctness, argv encoding, PID publishing, heartbeat mutex, overlay-build hard-fail, ImageHistory fake removal | BUG-756, 759, 763, 764, 765, 766, 767, 768, 769 |

## Pending work

### Round-9 manual-test crosswalk (in progress)

Per-test walk through [PLAN_ECS_MANUAL_TESTING.md](PLAN_ECS_MANUAL_TESTING.md) cross-referenced against [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md). Live working state: [docs/manual-test-spec-crosswalk.md](docs/manual-test-spec-crosswalk.md). Mismatches file as BUG-801..NNN under the Open section of [BUGS.md](BUGS.md). Coverage gaps (spec claims with no test) get added to the runbook at the end.

**Scope (in scope, not deferred):**

- ECS — Tracks A (49 tests), B (33 tests), C (11 tests), E (7 tests), F (12 tests), G (7 tests), I (9 tests).
- **Lambda — Track D (9 tests). Runs with a sockerless-lambda-bootstrap prebuilt overlay image** built from `agent/cmd/sockerless-lambda-bootstrap`, pushed to the Lambda ECR repo, and pointed at via `SOCKERLESS_LAMBDA_PREBUILT_OVERLAY_IMAGE`. D2-D7 (create/start/logs/exit-code/error/env) verify the function-invocation lifecycle. D8/D9 (exec/attach) run too — without `SOCKERLESS_CALLBACK_URL` they verify the spec's "NotImpl with named missing prerequisite" path, which is itself the testable behaviour.

**Skipped this round (separate work item):**

- Track H (podman-compose) — no `podman-compose` installed locally; trivial to add when `brew install podman-compose` is OK.
- Track J (runner integration) — needs a real GitLab Runner / GitHub Actions self-hosted runner; out of scope for one-laptop manual sweep.
- Tracks against GCP / Azure backends — need separate `terraform/environments/{cloudrun,aca,gcf,azf}/live` setups, none of which exist yet.

### Live-cloud validation runbooks

- **Phase 87 live-GCP** — GCP parallel to `scripts/phase86/*.sh`. Needs project + VPC connector.
- **Phase 88 live-Azure** — Azure parallel. Needs subscription + managed environment with VNet integration.
- **Phase 86 Lambda live track** — scripted already; deferred for session budget.

### Phase 68 — Multi-Tenant Backend Pools (queued)

Named pools of backends with scheduling and resource limits. `P68-001` done; 9 sub-tasks remaining (registry, router, limiter, lifecycle, metrics, scheduling, limits, tests, state save).

### Phase 78 — UI Polish (queued)

Dark mode, design tokens, error handling UX, container detail modal, auto-refresh, performance audit, accessibility, E2E smoke, documentation.

### Known workarounds to convert to real fixes

- **BUG-721** — SSM `acknowledge` format isn't accepted by the live AWS agent; backend dedupes retransmitted `output_stream_data` frames by MessageID. Proper fix requires live-AWS testing (Flags / PayloadDigest semantics). Pure sim path is unaffected.

### Phase 103 — Overlay-rootfs bootstrap mode (queued)

Replace the Phase 98 `find / -newer /proc/1` heuristic with a real overlay-FS-based diff/commit/cp/export path on every backend that runs through a sockerless bootstrap (Lambda, Cloud Run, ACA, GCF, AZF). The bootstrap mounts an overlayfs (`lowerdir=/, upperdir=…, workdir=…`), pivots into the merged dir, and execs the user command. The reverse-agent then reads `upper/` directly:

- `docker diff` returns the overlay's upper-dir entries — captures **deletions** as whiteouts (closes the BUG-750 known limitation).
- `docker commit` tars `upper/` directly — proper diff layer with whiteouts.
- `docker cp` and `docker export` stream from the merged FS or the upper layer — no `tar -cf -` over a `find` listing.

Gated behind `SOCKERLESS_OVERLAY_ROOTFS=1` per backend so existing deployments aren't disturbed. Out of scope: ECS — sockerless runs the operator's image as-is, so we can't insert a bootstrap there; ECS stays on Phase 102's SSM `find`/`tar` path.

Caveats per backend (need verification before wiring):
- **Lambda** — `CAP_SYS_ADMIN` available in the default execution context; workspace must live in `/tmp` (10 GB cap) since the function FS is read-only outside `/tmp`.
- **Cloud Run / GCF** — gVisor is the container runtime; overlayfs support is partial. May need a `tmpfs` upper-dir; fallback to Phase 98's `find`-based path if mount fails.
- **ACA / AZF** — full Linux containers; should work without caveats.

## Future ideas

- GraphQL subscriptions for real-time event streaming.
- Full GitHub App permission scoping.
- Webhook delivery UI.
- Cost controls (per-pool spending limits, auto-shutdown).
