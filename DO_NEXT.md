# Do Next

Roadmap [PLAN.md](PLAN.md) · status [STATUS.md](STATUS.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Branch

`phase-91b-backingmemory-ecs-lambda` — Phase 91b in flight (1 implementation commit + state save). Open a PR when ready; PR #147 already merged.

## Status

Phase 91b implementation is done on this branch. ACA done (clean EmptyDir match); ECS + AZF reject loudly with clear error messages; Lambda deferred to Phase 91c. After this lands, queue: Phase 91c (Lambda volume framework migration) → 91d (real pd-ephemeral lifecycle on cloudrun+gcf) → Phase 87c (optional zerolog→OTel bridge) → live-cloud validation track.

## Resume here — Phase 91b (BackingMemory ECS / ACA / AZF) — implementation done

Branch has 1 implementation commit + state save ready to PR:

1. `phase 91b: BackingMemory translator on ECS / ACA / AZF` — ACA gets clean `armappcontainers.Volume{StorageType: EmptyDir}`; ECS rejects loudly pointing at LinuxParameters.Tmpfs (cross-layer mismatch); AZF rejects loudly pointing at per-invocation /tmp. 5 new tests.

When ready: `git push -u origin phase-91b-backingmemory-ecs-lambda && gh pr create`. CI ~7 min.

**Why Lambda was deferred.** Lambda's `volumes.go::fileSystemConfigsForBinds` uses inline EFS predating the BackingSpec framework — never calls `storageBackings.Resolve`. Wiring `BackingMemory` requires migrating Lambda to the framework first. That's a separate refactor PR (Phase 91c) and shouldn't be bundled with translator extensions.

## Phase 91c — Lambda volume framework migration (next pickup)

**Goal.** Migrate Lambda's volume path from the pre-BackingSpec inline EFS pattern to the unified `storageBackings.Resolve` framework that ECS / ACA / AZF / cloudrun / gcf already use. Once migrated, `BackingMemory` rejection arrives for free (Lambda has no RAM-mount primitive — `/tmp` is per-invocation scratch).

**Files to touch.**

- `backends/lambda/volumes.go` — `fileSystemConfigsForBinds` currently builds `lambdatypes.FileSystemConfig` directly from `awscommon.EFSManager`. Wrap into a `BackingEFSEphemeral` driver path so the framework dispatches.
- `backends/lambda/volume_translator.go` (new) — same shape as ECS: per-bind resolve through registry, translate `BackingSpec` to `lambdatypes.FileSystemConfig`. Add `case core.BackingMemory:` rejection arm.
- `backends/lambda/server.go` — already registers `EFSEphemeralDriver` + `MemoryDriver`; verify nothing changes here.

**Test plan.** Mirror the ECS pattern: existing EFS path still works (regression guard); BackingMemory rejection error message points at `/tmp`.

## Phase 91d — Real pd-ephemeral lifecycle on cloudrun + gcf (later)

Sockerless-managed Compute Engine PD `disks.create`/`attach`/`delete` per task. Cloud Run Services don't expose PD volume attach as a first-class primitive — operator-side work + sim-side work. Multi-day cloud-API effort.

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
