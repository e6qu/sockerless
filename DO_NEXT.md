# Do Next

Roadmap [PLAN.md](PLAN.md) · status [STATUS.md](STATUS.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Branch

`phase-86-health-supervision` — Phase 86 work in flight (3 implementation commits + state save). Open a PR when ready; PR #143 already merged.

## Status

Phase 86 implementation is done on this branch. After it lands, **Phase 87 — centralized observability (Stack A)** is the next pickup. See bottom of this file for the Phase 87 brief — most of the design is already locked in PLAN.md.

## Resume here — Phase 86 (health + supervision surface) — implementation done

Branch has 3 implementation commits + state save ready to PR:

1. `phase 86: capture exit codes + bump probe timeout to 5s` — `start-component` records `.stack-pids/<n>.exit` on binary termination via a watcher subshell; `InstanceStatus` gains `Exit` + `CrashedSinceStart` fields; probe timeout 1 s → 5 s. 7 admin tests.
2. `phase 86: diagnostic endpoint bundling status + tail logs` — `GET /api/v1/topology/.../diagnostics?lines=N` returns status + last N lines (default 50, cap 1000) in one fetch. 6 endpoint tests.
3. `phase 86: UnhealthyDiagnosticPanel + per-row mount gate` — collapsible panel mounts under InstanceRow only when `shouldRender(status)` is true. Surfaces reason header + exit info + health_detail + last 50 log lines + deep links + refresh button. 10 vitest cases.

When ready: `git push -u origin phase-86-health-supervision && gh pr create`. CI ~7 min.

**What this does NOT change.** No auto-restart (explicitly deferred). No alerting / paging integration. No multi-instance health rollup (Phase 87's observability stack is the right place for that).

## Phase 87 — Centralized observability — Stack A (next pickup)

**Goal.** Every sockerless component (sim, backend, bleephub, admin) emits structured logs + traces to a local OpenTelemetry pipeline. Admin UI deep-links to per-instance log + trace queries. The Phase 86 file-tail-based diagnostic panel becomes one of two paths the UI offers — the file-tail stays for the no-OTel case; the OTel path activates when the operator opts in.

**Stack** (all Apache 2.0):

- OpenTelemetry Collector receives OTLP at `localhost:4317`, fans out: logs → VictoriaLogs OTLP HTTP, traces → Jaeger OTLP, optional metrics → VictoriaMetrics.
- VictoriaLogs UI on `:9428`, retention 7 d.
- Jaeger all-in-one UI on `:16686`, retention 72 h.

**Invariant preserved.** Components emit OTLP only when `OTEL_EXPORTER_OTLP_ENDPOINT` is set in their env. Unset = today's behaviour (zerolog → stdout). No admin coupling, no required env var, no startup registration.

**Sub-steps (already in PLAN.md — copying for convenience):**

1. `backends/core/otel/` — wraps `go.opentelemetry.io/otel` SDK setup (logs + traces + resource attrs). Reads `OTEL_EXPORTER_OTLP_ENDPOINT` + service name. Used by every component's `main.go` in 3 lines.
2. HTTP middleware — wrap each backend / sim mux with `otelhttp.NewHandler` so spans land per request automatically.
3. zerolog → OTel logs bridge — existing `zerolog.Logger` calls also export to the OTel logs SDK. No log-line API changes.
4. `make stack-observability-{up,down,status}` in `make/stack.mk`. Runs collector + VictoriaLogs + Jaeger as background processes; PIDs in `.stack-pids/observability/`. Default config emits to `./.sockerless-state/observability/{logs,traces}/` with rotation + 5 GB total cap.
5. Admin UI integration — per-instance "View logs" + "View traces" deep links (filter by `service.name = <instance-name>`). Inline log tail (Phase 81) + diagnostic panel (Phase 86) still work for the no-OTel path.
6. Documentation — `ui/README.md` + new `docs/OBSERVABILITY.md` cover both modes.

**If Stack A turns out unsuitable.** Same component code (OTLP) works against OpenObserve (AGPL) or SigNoz (MIT) — only `make/stack.mk` changes. The component-side bridge is OTel SDK, not collector-specific.

**Files to touch (rough):**

- `backends/core/otel/setup.go` (new) — SDK wrapper.
- `backends/core/otel/zerolog_bridge.go` (new) — logs bridge.
- Each `*/main.go` (admin, sims × 3, backends × 7, bleephub) — 3-line OTel init at startup.
- `make/stack.mk` — `stack-observability-{up,down,status}` targets.
- `cmd/sockerless-admin/api_topology_diagnostics.go` — extend to include OTel deep links when endpoint env is set.
- `ui/packages/admin/src/components/UnhealthyDiagnosticPanel.tsx` — render OTel deep links when surfaced.
- `docs/OBSERVABILITY.md` (new).

## Invariants (re-state on every commit)

- **Components stay decoupled.** No admin-required env vars on sims/backends/bleephub. Admin reads only what they already expose (`/v1/health`, `/v1/info`, env vars). For Phase 87 observability: components emit OTLP only when `OTEL_EXPORTER_OTLP_ENDPOINT` is set in their env. Unset = today's stdout behaviour.
- **No fallbacks.** Unknown config values fail-loud. No silent defaults.
- **CI green per commit.** Each commit must be independently testable.
- **Test target gating.** All backend integration tests require `SOCKERLESS_TEST_TARGET=sim|cloud` (no skip).
- **No docs-only PRs.** Pair docs updates with implementation work on the same branch / PR.

## Roadmap

Phases 79.2 → 80 → 81 → 82 ✓ all in #140. Next: 83 → 84 → 85 → 86 → 87. After 87: 91–94 (real per-cloud volume provisioning) + the live-cloud validation track (Lambda live, Cloud Run Services / ACA Apps live, AZF cloud-dns live, Lambda service-mesh live, ACA/AZF Azure AD live). See [PLAN.md](PLAN.md) for sub-steps. Will likely split into multiple PRs once natural seams appear (e.g. Phase 87 — observability — is independent and can land standalone).
