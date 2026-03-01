# Sockerless — Next Steps

## Current State

Phases 1-67, 69-74 complete. **664 tasks done across 74 phases.** Phase 68 (Multi-Tenant Backend Pools) paused after P68-001.

## Next: Phase 75 — Simulator Dashboards (AWS, GCP, Azure)

Add dashboards to cloud simulators showing simulated resources. Browser calls simulator's own cloud APIs (same-origin).

**13 tasks** (P75-001 → P75-013): Simulator SPA handler, shared components, AWS/GCP/Azure SPAs with API clients and embed files, tests.

## After Phase 75

| Phase | Description | Depends on |
|---|---|---|
| 76 | bleephub Dashboard — GitHub Actions (11 tasks) | 74 |
| 77 | gitlabhub Dashboard — GitLab CI (10 tasks) | 74 |
| 78 | Polish, Dark Mode, Cross-Component UX (10 tasks) | 75, 76, 77 |

Phases 75, 76, 77 are independent and can be done in any order after 74.

## Test Commands Reference

```bash
# Unit + integration
make test

# UI build + test
make ui-build    # builds all 10 SPAs + copies dist/
make ui-test     # runs Vitest (16 tests)

# Per-backend build with UI
make build-memory-with-ui
make build-ecs-with-ui
make build-docker-backend-with-ui
make build-frontend-with-ui

# Per-backend build without UI (CI mode)
make build-memory-noui
make build-ecs-noui

# Lint all 15 modules
make lint
```
