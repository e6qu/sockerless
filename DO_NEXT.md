# Do Next

Roadmap [PLAN.md](PLAN.md) · status [STATUS.md](STATUS.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Branch

`phase-91-pd-ephemeral-volumes` — Phase 91 in flight (1 implementation commit + state save). Open a PR when ready; PR #146 already merged.

## Status

Phase 91 implementation is done on this branch. After it lands, follow-ups in order: Phase 91b (BackingMemory on ECS+Lambda) → 91c (BackingMemory on ACA+AZF) → 91d (real pd-ephemeral lifecycle on cloudrun+gcf) → Phase 87c (optional zerolog→OTel bridge) → live-cloud validation track.

## Resume here — Phase 91 (BackingMemory cloudrun + gcf) — implementation done

Branch has 1 implementation commit + state save ready to PR:

1. `phase 91: BackingMemory translator on cloudrun + gcf` — adds `case core.BackingMemory` arm to both translators, mapping to `EmptyDir{Medium: MEMORY}` with `SizeLimit` from `spec.Memory.SizeMB`. 5 unit tests.

When ready: `git push -u origin phase-91-pd-ephemeral-volumes && gh pr create`. CI ~7 min.

**Audit-driven scope.** The original Phase 91 brief was "lift `emptyDir` fallback to `pd-ephemeral` / `efs-ephemeral` / `azure-files-ephemeral`". Reading the existing code:

- `efs-ephemeral` is already wired on ECS (Phase 127). Lambda's inline EFS path predates the BackingSpec framework.
- `azure-files-ephemeral` is already wired on ACA + AZF.
- `pd-ephemeral` on Cloud Run is bookmarked — Cloud Run Services lack first-class PD attach (`specs/CLOUD_RESOURCE_MAPPING.md` line 567-568). Real implementation requires multi-day Compute Engine API lifecycle work and is queued as Phase 91d.

The actual audit-discovered gap was `BackingMemory`: Phase 127 added the driver + registered it in all 6 backends, but no translator handled the case. This PR closes that gap on cloudrun + gcf.

## Phase 91b — BackingMemory on ECS + Lambda (next pickup)

**Goal.** Same translator extension on the AWS backends.

**Files to touch.**

- `backends/ecs/volume_translator.go` — add `case core.BackingMemory` mapping to `ecstypes.Volume{Name, Host{}}` with the container-side mount as `LinuxParameters.Tmpfs[]` for true RAM-backed mount. Trade-off: ECS volumes are at the task-def layer; tmpfs is at the container-def layer. Decide whether `BackingMemory` on ECS emits a Volume (host-vol shape) or rejects with "use container tmpfs" error.
- `backends/lambda/...` — Lambda has no real volume primitive for ephemeral RAM-backed mounts; `/tmp` is per-invocation scratch (512 MB–10 GB). `BackingMemory` may have to reject with a clear error pointing at /tmp.

**Test plan.** Mirror the cloudrun/gcf tests. ECS test expects either Volume or specific error. Lambda test expects rejection.

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
