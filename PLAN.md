# Sockerless — Roadmap

> 85 phases complete (756 tasks). 583 bugs fixed, 0 open.
>
> **Goal:** Replace Docker Engine with Sockerless for any Docker API client — `docker run`, `docker compose`, TestContainers, CI runners — backed by real cloud infrastructure (AWS, GCP, Azure).

## Guiding Principles

1. **Docker API fidelity** — match Docker's REST API exactly
2. **Real execution** — simulators and backends actually run commands
3. **External validation** — proven by unmodified external test suites
4. **No new frontend abstractions** — Docker REST API is the only interface
5. **Driver-first handlers** — all handler code through driver interfaces
6. **LLM-editable files** — source files under 400 lines
7. **GitHub API fidelity** — bleephub works with unmodified `gh` CLI
8. **State persistence** — every task ends with state save

---

## Phase 68 — Multi-Tenant Backend Pools (In Progress)

Named pools of backends with scheduling and resource limits.

| Task | Status | Description |
|---|---|---|
| P68-001 | done | Pool configuration (JSON config) |
| P68-002 | | Pool registry (in-memory, each with own BaseServer + Store) |
| P68-003 | | Request router (route by label or default pool) |
| P68-004 | | Concurrency limiter (per-pool semaphore, 429 on overflow) |
| P68-005 | | Pool lifecycle (create/destroy at runtime via management API) |
| P68-006 | | Pool metrics (per-pool stats on `/internal/metrics`) |
| P68-007 | | Round-robin scheduling (multi-backend pools) |
| P68-008 | | Resource limits (max containers, max memory per pool) |
| P68-009 | | Unit + integration tests |
| P68-010 | | Save final state |

---

## Phase 78 — UI Polish

Dark mode, design tokens, error handling UX, container detail modal, auto-refresh, performance audit, accessibility, E2E smoke, documentation.

---

## Future Ideas

- GraphQL subscriptions for real-time event streaming
- Full GitHub App permission scoping
- Webhook delivery UI
- Cost controls (per-pool spending limits, auto-shutdown)
