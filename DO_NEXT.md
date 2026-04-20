# Do Next

Snapshot pointer for the next session. Updated after every task per user directive.

## Phase 89 — Stateless backend audit (substantially complete)

11 commits landed on branch `post-phase86-continuation`. 3 of 4 Phase 89 bugs fully fixed; 1 partial (BUG-724 blocked on Phase 87/88).

| Bug | Status |
|---|---|
| BUG-723 | fixed — Store.Images disk persistence removed; `docker images` cloud-derived across all 6 cloud backends via `CloudImageLister` (ECR for AWS; `core.OCIListImages` for GCP/Azure). |
| BUG-724 | partial — `CloudPodLister` interface + ECS `ListPods` landed; cloudrun+ACA pod listing blocked on Phase 87/88. |
| BUG-725 | fixed — `resolve*State` cache+cloud-fallback helpers across 4 backends; every cloud-state-dependent callsite migrated; unit tests for cache-hit + cache-miss paths. |
| BUG-726 | fixed — `resolveNetworkState` across ECS + Cloud Run + ACA; Cloud Map namespaces tagged `sockerless:network-id`. |

Remaining Phase 89 polish (non-blocking):
- Per-backend restart-resilience integration tests against simulators (ECS/Cloud Run/ACA/Lambda). Unit tests already prove the helpers; integration tests prove the SDK interaction.

## Open work that unblocks Phase 89 BUG-724 fully

- **Phase 87** — cloudrun Jobs → Cloud Run Services with internal ingress + VPC connector. Closes BUG-715 (cross-container DNS) AND unlocks pod sidecars for BUG-724. Multi-day architectural rewrite.
- **Phase 88** — aca Jobs → ACA Apps with internal ingress. Same shape for Azure. Closes BUG-716 AND unlocks pod sidecars for BUG-724.

## Phase 86 Phase C — CLOSED 2026-04-20

ECS backend live-validated end-to-end. AWS infra torn down, zero residue. See `WHAT_WE_DID.md` for the session log.

## Queued (not in progress)

- **Phase 68** — Multi-Tenant Backend Pools (P68-002 → 010). Orthogonal to Phase 87/88/89.
- **Phase 78** — UI Polish (dark mode, design tokens, etc.)
- **Phase 86 Lambda live track** — was deferred at Phase C closure; no architectural blockers.

## Branch state

`post-phase86-continuation` — 15 commits ahead of `origin/main`. All hooks pass. Ready for PR, or continue stacking more commits.

## Operational state

- AWS: zero residue (state buckets + DDB lock table retained as cheap reusable infra).
- Local sockerless backend: stopped.
- No credentials in environment.

## Phase 89 spec

`specs/CLOUD_RESOURCE_MAPPING.md` — canonical docker concept → cloud resource mapping for all 7 backends, state-derivation rules, recovery contract, current status per bug. Cross-linked from `specs/SOCKERLESS_SPEC.md` table-of-contents and `specs/BACKEND_STATE.md`.
