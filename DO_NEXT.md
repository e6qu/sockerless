# Do Next

Roadmap [PLAN.md](PLAN.md) · status [STATUS.md](STATUS.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Branch

`phase-87-observability` — Phase 87 work in flight (4 implementation commits + state save). Open a PR when ready; PR #144 already merged.

## Status

Phase 87 first-PR implementation is done on this branch. After it lands, **Phase 87b — component-side OTel SDK wiring** is the next pickup. See bottom of this file for the Phase 87b brief.

## Resume here — Phase 87 (observability — Stack A) — implementation done

Branch has 4 implementation commits + state save ready to PR:

1. `phase 87: stack-observability make targets + collector config` — `make stack-observability-{up,down,status}` brings up otel-collector-contrib + VictoriaLogs + Jaeger as background processes. Default collector config scrapes `.stack-pids/*.log` via filelog receiver so logs flow without binary changes.
2. `phase 87: GET /api/v1/observability config endpoint` — admin reads `OTEL_LOGS_DASHBOARD` / `OTEL_TRACES_DASHBOARD` env vars at boot; returns `{enabled, logs_dashboard, traces_dashboard, ...}` so the UI knows when to render deep-link chips. 7 unit tests.
3. `phase 87: VictoriaLogs / Jaeger deep links in diagnostic panel` — `<UnhealthyDiagnosticPanel>` fetches `/api/v1/observability` (cached) and renders chips when enabled; falls back to file-tail-only when disabled. 2 new vitest cases.
4. `phase 87: docs/OBSERVABILITY.md` — two-mode operator guide.

When ready: `git push -u origin phase-87-observability && gh pr create`. CI ~7 min.

**What this does NOT change.** No SDK wiring on components — Phase 87 only ships the *stack* + admin integration. Logs work day-1 via the filelog receiver scraping `.stack-pids/*.log`. Traces require Phase 87b component-side wiring.

## Phase 87b — Component-side OTel SDK wiring (next pickup)

**Goal.** Each component's `main.go` initialises the OTel SDK when `OTEL_EXPORTER_OTLP_ENDPOINT` is set in its env, then wraps its mux with `otelhttp.NewHandler` for per-request spans. zerolog log lines also flow via OTel logs SDK so OTLP-mode operators don't need the filelog receiver fallback.

**Design.** Existing `backends/core/otel.go` already has `InitTracer(serviceName) (shutdown, error)` (used by `backends/docker/cmd/main.go`). Phase 87b extends:

1. Rename `InitTracer` → `InitObservability` returning a multi-shutdown function. Add OTel logs SDK setup alongside the existing trace SDK setup.
2. New `backends/core/otel_zerolog.go`: zerolog Hook that mirrors every log entry to the OTel logs provider. zerolog API doesn't change for callers.
3. Per-component `main.go` updates (3 lines each): `shutdown := core.InitObservability("<service-name>"); defer shutdown(ctx)`.
4. Per-component `mux := http.NewServeMux(); handler := otelhttp.NewHandler(mux, "sockerless-<service>")`.
5. Sims + admin + bleephub: same shape but they don't import `backends/core` today. Either:
   - Add a new top-level `pkg/otel` module that all of them import (cleanest), or
   - Duplicate the small init helper in each module (matches the per-cloud `shared/` pattern).

**Files to touch.** ~12 `main.go` files (admin + 3 sims + 7 backends + bleephub). One new shared package or three duplicates of the init helper.

**Out of scope still.** Per-binary metrics export (counters / histograms). Custom span attributes beyond what `otelhttp` adds automatically.


## Invariants (re-state on every commit)

- **Components stay decoupled.** No admin-required env vars on sims/backends/bleephub. Admin reads only what they already expose (`/v1/health`, `/v1/info`, env vars). For Phase 87 observability: components emit OTLP only when `OTEL_EXPORTER_OTLP_ENDPOINT` is set in their env. Unset = today's stdout behaviour.
- **No fallbacks.** Unknown config values fail-loud. No silent defaults.
- **CI green per commit.** Each commit must be independently testable.
- **Test target gating.** All backend integration tests require `SOCKERLESS_TEST_TARGET=sim|cloud` (no skip).
- **No docs-only PRs.** Pair docs updates with implementation work on the same branch / PR.

## Roadmap

Phases 79.2 → 80 → 81 → 82 ✓ all in #140. Next: 83 → 84 → 85 → 86 → 87. After 87: 91–94 (real per-cloud volume provisioning) + the live-cloud validation track (Lambda live, Cloud Run Services / ACA Apps live, AZF cloud-dns live, Lambda service-mesh live, ACA/AZF Azure AD live). See [PLAN.md](PLAN.md) for sub-steps. Will likely split into multiple PRs once natural seams appear (e.g. Phase 87 — observability — is independent and can land standalone).
