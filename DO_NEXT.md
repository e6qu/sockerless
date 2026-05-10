# Do Next

Roadmap [PLAN.md](PLAN.md) · status [STATUS.md](STATUS.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Branch

`phase-87b-component-otel-wiring` — Phase 87b in flight (2 implementation commits + state save). Open a PR when ready; PR #145 already merged.

## Status

Phase 87b implementation is done on this branch. After it lands, the optional **Phase 87c — zerolog → OTel logs bridge** is available, then phases 91–94 (volume provisioning) + live-cloud validation track.

## Resume here — Phase 87b (component-side OTel SDK wiring) — implementation done

Branch has 2 implementation commits + state save ready to PR:

1. `phase 87b: wire core.InitTracer into 6 backend main.go files` — ecs / lambda / cloudrun / gcf / aca / azf each gain a 4-line OTel init at startup. otelhttp middleware was already in `backends/core/server.go`; this commit makes it actually emit by initialising the tracer.
2. `phase 87b: wire OTel SDK + otelhttp on sims + admin` — 3 sims gain `shared/otel.go` + outermost `otelhttp.NewHandler` + 4-line init in main.go. Admin gains a duplicated InitTracer helper (separate Go module without backend-core dep) + otelhttp wrap on the mux. 11 new tracer tests.

When ready: `git push -u origin phase-87b-component-otel-wiring && gh pr create`. CI ~7 min.

bleephub already wired since Phase 86 baseline — no changes there.

**What this does NOT change.** No zerolog → OTel logs bridge — operators in OTLP mode rely on the collector's filelog receiver scraping `.stack-pids/*.log` (Phase 87). Bridge is the optional Phase 87c if operators want OTLP-only emission.

## Phase 87c — zerolog → OTel logs bridge (optional next pickup)

**Goal.** Each component's zerolog calls also export to the OTel logs SDK so OTLP-mode operators don't need the filelog receiver fallback. Bridge is *optional* — the filelog receiver path from Phase 87 covers logs without binary changes; the bridge is for operators who want a single OTLP transport.

**Design.** Each component creates a zerolog hook that mirrors every event to the OTel logs provider. zerolog API doesn't change for callers — same `logger.Info().Str("k", "v").Msg("...")` shape.

**Files to touch.** Same 4 modules Phase 87b touched (backends/core + sim shared × 3 + admin) plus bleephub. New `otel_zerolog.go` per module + 1 line in each Init wiring to register the hook.

**Out of scope still.** Per-binary metrics export (counters / histograms). Custom span attributes beyond what `otelhttp` adds automatically.


## Invariants (re-state on every commit)

- **Components stay decoupled.** No admin-required env vars on sims/backends/bleephub. Admin reads only what they already expose (`/v1/health`, `/v1/info`, env vars). For Phase 87 observability: components emit OTLP only when `OTEL_EXPORTER_OTLP_ENDPOINT` is set in their env. Unset = today's stdout behaviour.
- **No fallbacks.** Unknown config values fail-loud. No silent defaults.
- **CI green per commit.** Each commit must be independently testable.
- **Test target gating.** All backend integration tests require `SOCKERLESS_TEST_TARGET=sim|cloud` (no skip).
- **No docs-only PRs.** Pair docs updates with implementation work on the same branch / PR.

## Roadmap

Phases 79.2 → 80 → 81 → 82 ✓ all in #140. Next: 83 → 84 → 85 → 86 → 87. After 87: 91–94 (real per-cloud volume provisioning) + the live-cloud validation track (Lambda live, Cloud Run Services / ACA Apps live, AZF cloud-dns live, Lambda service-mesh live, ACA/AZF Azure AD live). See [PLAN.md](PLAN.md) for sub-steps. Will likely split into multiple PRs once natural seams appear (e.g. Phase 87 — observability — is independent and can land standalone).
