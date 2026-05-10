# Do Next

Roadmap [PLAN.md](PLAN.md) · status [STATUS.md](STATUS.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Branch

`phase-87c-zerolog-otel-bridge` — Phase 87c full scope on PR #150 (open). 7 backends + 3 sims + bleephub + admin all bridged. Awaiting CI + user merge.

## Status

Phase 87c (consolidated, full scope per user direction "keep all phase 87 on a single PR") is implementation-complete on this branch. After this merges, queue: Phase 91d (real pd-ephemeral lifecycle on cloudrun+gcf) → live-cloud validation track.

## Resume here — Phase 87c (full scope) on PR #150

Branch has 2 implementation commits + state save ready/landed:

1. `phase 87c: zerolog OTel bridge across 7 backends` — `backends/core/otel.go` gains `InitObservability` + `OTelLogWriter`. 7 backend `main.go` use `MultiLevelWriter(consoleW, obs.LogWriter)`.
2. `phase 87c: zerolog OTel bridge across sims + admin + bleephub` — bridge mirrored into `simulators/{aws,gcp,azure}/shared/otel.go` (with `Config.LogWriter` plumbed through `NewServer`), `bleephub/otel.go`, `cmd/sockerless-admin/otel.go` (adds `TextLogWriter` for stdlib `log`).

CI running on PR #150. **Do NOT auto-merge — wait for user.**

## Phase 91d — Real pd-ephemeral lifecycle on cloudrun + gcf (later)

Sockerless-managed Compute Engine PD `disks.create`/`attach`/`delete` per task. Cloud Run Services don't expose PD volume attach as a first-class primitive — operator-side work + sim-side work. Multi-day cloud-API effort.


## Invariants (re-state on every commit)

- **Components stay decoupled.** No admin-required env vars on sims/backends/bleephub. Admin reads only what they already expose (`/v1/health`, `/v1/info`, env vars). For Phase 87 observability: components emit OTLP only when `OTEL_EXPORTER_OTLP_ENDPOINT` is set in their env. Unset = today's stdout behaviour.
- **No fallbacks.** Unknown config values fail-loud. No silent defaults.
- **CI green per commit.** Each commit must be independently testable.
- **Test target gating.** All backend integration tests require `SOCKERLESS_TEST_TARGET=sim|cloud` (no skip).
- **No docs-only PRs.** Pair docs updates with implementation work on the same branch / PR.

## Roadmap

Phases 79.2 → 80 → 81 → 82 ✓ all in #140. Next: 83 → 84 → 85 → 86 → 87. After 87: 91–94 (real per-cloud volume provisioning) + the live-cloud validation track (Lambda live, Cloud Run Services / ACA Apps live, AZF cloud-dns live, Lambda service-mesh live, ACA/AZF Azure AD live). See [PLAN.md](PLAN.md) for sub-steps. Will likely split into multiple PRs once natural seams appear (e.g. Phase 87 — observability — is independent and can land standalone).
