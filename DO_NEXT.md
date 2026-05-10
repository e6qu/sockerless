# Do Next

Roadmap [PLAN.md](PLAN.md) · status [STATUS.md](STATUS.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Branch

`phase-91c-lambda-backingspec-migration` — Phase 91 consolidation in flight (2 implementation commits + state save). Open a PR when ready; PR #148 already merged.

## Status

Phase 91 (consolidated) implementation is done on this branch. Per user direction, all remaining 91 work landed here as one PR — no more sub-phase splits. After this lands, queue: Phase 91d (real pd-ephemeral lifecycle on cloudrun+gcf) → Phase 87c (optional zerolog→OTel bridge) → live-cloud validation track.

## Resume here — Phase 91 (consolidated) — implementation done

Branch has 2 implementation commits + state save ready to PR:

1. `phase 91c: Lambda volume_translator.go scaffolding + BackingMemory reject` — per-bind translator dispatch shape mirroring ECS / ACA / AZF / cloudrun / gcf. `BackingEFSEphemeral` → `FileSystemConfig{Arn, LocalMountPath}`; `BackingMemory` → loud reject; unknown → generic. 5 tests.
2. `phase 91 (consolidated): Lambda framework migration + GCP PD reject + ECR Gallery` — three coupled changes:
   - Lambda's `fileSystemConfigsForBinds` migrated onto `s.storageBackings.Resolve(BackingEFSEphemeral) → translator`. Lambda joins the other 5 backends in framework dispatch.
   - cloudrun + gcf reject `BackingPDEphemeral` with concrete pointers (`gcs-fuse` MountOptions per BUG-944, `gcs-sync`, GCE-backend bookmark per spec line 567). 2 tests.
   - Cloudrun + gcf integration TestMain switched to `public.ecr.aws/docker/library/{alpine,golang}` to dodge Docker Hub anonymous-pull throttling (saved as memory feedback_ecr_gallery_alt.md).

When ready: `git push -u origin phase-91c-lambda-backingspec-migration && gh pr create`. CI ~7 min.

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
