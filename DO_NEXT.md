# Do Next

Roadmap [PLAN.md](PLAN.md) · status [STATUS.md](STATUS.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Branch

`phase-87d-92-observability-closeout-gcs-sync` — Phase 87 closeout (87d) + Phase 92 (gcs-fuse → gcs-sync) bundled. 6 commits ready to push as a PR. **Do NOT auto-merge — wait for user.**

## Status

Phase 87d closes the three remaining gaps the audit surfaced after PR #150 merged: trace propagation across admin + bleephub HTTP clients, MeterProvider + runtime metrics across all 12 components, and a `make stack-observability-validate` end-to-end harness. Phase 92 ships BUG-987: deregister `Backing: gcs-fuse` on cloudrun + gcf because Cloud Run rejects the cache-TTL flags it needs to be safe across tasks; translator points operators at `gcs-sync` instead.

## Resume here — Phase 87d + 92 bundle

Branch has 6 commits ready to push:

1. `phase 87d: trace context propagation across admin + bleephub HTTP clients` — 9 client construction sites wrapped with otelhttp.NewTransport; global propagator set inside InitObservability.
2. `phase 87d: OTel MeterProvider + runtime metrics across all components` — 5 InitObservability impls extended; runtime.Start emits Go runtime metrics; 2 new core tests.
3. `phase 87d: stack-observability-validate make target` — manual operator-grade harness in make/stack.mk + docs/OBSERVABILITY.md § Validation.
4. `phase 92: deregister gcs-fuse on cloudrun + gcf, reject in translator (BUG-944)` — registry change + translator reject arms + 2 new tests + reused PD-ephemeral test cleanup.
5. `phase 92: docs + BUG-944 closure` — specs/CLOUD_RESOURCE_MAPPING.md + BUGS.md (BUG-987 = 987 filed/fixed).
6. `docs: state save for Phase 87d + 92` — this commit.

CI runs after `git push -u origin phase-87d-92-observability-closeout-gcs-sync && gh pr create`. **Do NOT auto-merge — wait for user.**

## Phase 91d — Real pd-ephemeral lifecycle on cloudrun + gcf

**Bookmarked indefinitely.** `runpb.Volume` protobuf has no PersistentDisk field; Cloud Run Admin API doesn't expose PD attach as a first-class primitive. Implementation requires either a future GCE-style sockerless backend or a Cloud Run feature change. Reject-with-pointers shape (Phase 91c) stays in place.


## Invariants (re-state on every commit)

- **Components stay decoupled.** No admin-required env vars on sims/backends/bleephub. Admin reads only what they already expose (`/v1/health`, `/v1/info`, env vars). For Phase 87 observability: components emit OTLP only when `OTEL_EXPORTER_OTLP_ENDPOINT` is set in their env. Unset = today's stdout behaviour.
- **No fallbacks.** Unknown config values fail-loud. No silent defaults.
- **CI green per commit.** Each commit must be independently testable.
- **Test target gating.** All backend integration tests require `SOCKERLESS_TEST_TARGET=sim|cloud` (no skip).
- **No docs-only PRs.** Pair docs updates with implementation work on the same branch / PR.

## Roadmap

Phases 79.2 → 80 → 81 → 82 ✓ all in #140. Next: 83 → 84 → 85 → 86 → 87. After 87: 91–94 (real per-cloud volume provisioning) + the live-cloud validation track (Lambda live, Cloud Run Services / ACA Apps live, AZF cloud-dns live, Lambda service-mesh live, ACA/AZF Azure AD live). See [PLAN.md](PLAN.md) for sub-steps. Will likely split into multiple PRs once natural seams appear (e.g. Phase 87 — observability — is independent and can land standalone).
