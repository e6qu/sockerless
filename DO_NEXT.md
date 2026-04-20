# Do Next

Snapshot pointer for the next session. Updated after every task per user directive.

## Phase 86 Phase C — CLOSED 2026-04-20

ECS backend live-validated end-to-end. AWS infra torn down, zero residue. Branch `post-phase86-continuation` ready for PR (commit `fc8d25d` is the in-progress checkpoint; need to land BUG-717/719/720/721/722 + Phase 89 doc updates as a follow-up commit before PR).

Tested live (eu-west-1, account 729079515331) and verified PASS:
- 2.1 `docker run --rm alpine echo`
- 2.2 `docker run -d` + `docker logs`
- 2.3 cross-container DNS — FQDN AND short name
- 2.4 `docker exec` — single line, no garbage

## Bugs from this session

13 fully fixed: 708, 709, 710, 711, 712, 713, 714, 717, 718, 719, 720, 721, 722.
2 split into Phase 87 (cloudrun rewrite for 715) + Phase 88 (ACA rewrite for 716).
4 split into Phase 89 (stateless audit for 723, 724, 725, 726).

## Next phase candidates (user choice)

### Phase 89 — Stateless backend audit + cloud-resource mapping (recommended next)

Closes BUG-723/724/725/726. The user's directive: "backends should be stateless; state derived from cloud's actuals; ECS tasks → containers/pods, sockerless-tagged SG + Cloud Map ns → docker network".

Concrete deliverables:
1. `docs/CLOUD_RESOURCE_MAPPING.md` — formal mapping per cloud (ECS task → container/pod, GCP Cloud Run Service → container, etc.)
2. State derivation refactor in each backend — replace in-memory state stores with on-demand cloud queries (caches allowed but invalidatable, never source of truth)
3. Remove `Store.Images` disk persistence — query the cloud registry instead
4. Backend recovery must work after restart with no on-disk or in-memory state

### Phase 87 — cloudrun Jobs → Services with internal ingress (BUG-715)

Multi-day. Substantial backend rewrite. Closes BUG-715 + BUG-713's underlying cause (cross-container DNS via Cloud DNS A-records is broken on Jobs).

### Phase 88 — ACA Jobs → Apps with internal ingress (BUG-716)

Multi-day. Same shape as Phase 87 for Azure.

### Phase 86 Lambda track

Provision Lambda infra in us-east-1, run Lambda baseline + agent-as-handler. Was deferred this session for time reasons; zero blockers.

### Phase 68 — Multi-Tenant Backend Pools (P68-002 → 010)

Already on the roadmap; orthogonal to Phase 87/88/89.

### Phase 78 — UI Polish

Dark mode, design tokens, etc.

## Operational state

- AWS account 729079515331: zero sockerless residue (state buckets + DDB lock table retained).
- Local sockerless backend: stopped (port :3375 free).
- Branch: `post-phase86-continuation`. Last commit `fc8d25d` (Phase C session 2 fixes 708-714+717+718). Need to land BUG-717/719-722 + state-file updates as a follow-up commit.
