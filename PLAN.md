# Sockerless — Roadmap

> Phases 1-67, 69-77, 79-82 complete (725 tasks). Phase 68 in progress. 311 bugs fixed (27 sprints), 0 open.
>
> **Production target:** Replace Docker Engine with Sockerless for any Docker API client — `docker run`, `docker compose`, TestContainers, CI runners (GitHub Actions, GitLab CI) — backed by real cloud infrastructure (AWS, GCP, Azure).

## Guiding Principles

1. **Docker API fidelity** — The frontend must match Docker's REST API exactly. CI runners should not need patching.
2. **Real execution** — Simulators and backends must actually run commands and produce real output. Synthetic/echo mode is a last resort.
3. **External validation** — Correctness is proven by running unmodified external test suites (`act`, `gitlab-runner`, `gitlab-ci-local`, upstream act, `actions/runner`, `gh` CLI, gitlabhub gitlab-runner).
4. **No new frontend abstractions** — The Docker REST API is the only interface. No Kubernetes, no Podman, no custom APIs.
5. **Driver-first handlers** — All handler code must operate through driver interfaces, never through direct `Store.Processes` access or `ProcessFactory` checks.
6. **LLM-editable files** — Keep source files under 400 lines.
7. **GitHub API fidelity** — bleephub must match the real GitHub API closely enough that unmodified `gh` CLI commands work against it.
8. **State persistence** — Every task must end with a state save: update `PLAN.md`, `STATUS.md`, `WHAT_WE_DID.md`, `MEMORY.md`, and `_tasks/done/`.

---

## Completed Phases (1-82)

See `WHAT_WE_DID.md` for details and `_tasks/done/` for per-task logs.

| Phase | Summary |
|---|---|
| 1-56 | Foundation: 3 simulators, 8 backends, agent, frontend, WASM sandbox, bleephub, CLI, pods, Docker API |
| 57-67 | CI runners (GitHub Actions + GitLab CI), API hardening, webhooks, GitHub Apps, OTel, network isolation |
| 69-72 | ARM64, simulator fidelity, SDK/CLI verification, full-stack E2E |
| 73-77 | UI: 13 SPAs (Bun/Vite/React 19), bleephub + gitlabhub dashboards, LogViewer |
| 79-82 | Admin: dashboard, docs, process management, project bundles |

---

## Phase 68 — Multi-Tenant Backend Pools (In Progress)

**Goal:** Named pools of backends with scheduling and resource limits. Each pool has a backend type, concurrency limit, and queue. Requests are routed to pools by label or explicit selection.

### P68-A: Pool Infrastructure (~6 tasks)

| Task | Status | Description |
|---|---|---|
| P68-001 | ✅ | **Pool configuration** — JSON config: pool name, backend type, max concurrency, queue size |
| P68-002 | | **Pool registry** — in-memory registry of pools, each with its own `BaseServer` + `Store` |
| P68-003 | | **Request router** — route Docker API requests to pools by label (`com.sockerless.pool`) or default pool |
| P68-004 | | **Concurrency limiter** — per-pool semaphore, queue overflow returns 429 |
| P68-005 | | **Pool lifecycle** — create/destroy pools at runtime via management API |
| P68-006 | | **Pool metrics** — per-pool container count, queue depth, utilization exposed on `/internal/metrics` |

### P68-B: Scheduling + Tests (~4 tasks)

| Task | Status | Description |
|---|---|---|
| P68-007 | | **Round-robin scheduling** — distribute requests across pool instances when pool has multiple backends |
| P68-008 | | **Resource limits** — per-pool max containers, max total memory (advisory, enforced at scheduling time) |
| P68-009 | | **Unit + integration tests** — pool CRUD, routing, concurrency limits, queue overflow, metrics |
| P68-010 | | **Save final state** |

---

## Phase 78 — Polish, Dark Mode, Cross-Component UX

**Goal:** Cross-cutting UI polish: dark mode, error handling, accessibility, performance, documentation.

| Task | Status | Description |
|---|---|---|
| P78-001 | | **Dark mode** — Tailwind class-based strategy, localStorage preference, toggle in header |
| P78-002 | | **Design system tokens** — Shared color palette, spacing, typography, per-component theme variants |
| P78-003 | | **Error handling UX** — Connection lost indicator, retry buttons, stale data warnings |
| P78-004 | | **Container detail modal** — Click container → inspect data, streaming logs, actions |
| P78-005 | | **Auto-refresh controls** — Global toggle, configurable interval, Page Visibility API pause |
| P78-006 | | **Performance audit** — Bundle size < 200KB gzipped per SPA, code splitting, build time < 30s |
| P78-007 | | **Accessibility** — Keyboard nav, ARIA labels, color contrast (light + dark) |
| P78-008 | | **E2E smoke test** — Go test: start memory backend, fetch `/ui/`, verify React root + API coexistence |
| P78-009 | | **Documentation** — `ui/README.md`: dev setup, architecture, component catalog. Update root README |
| P78-010 | | **Final state save** |

---

## Future Ideas (Not Scheduled)

- **WASI Preview 2** — component model with async I/O; would enable real subprocesses in WASM sandbox
- **GraphQL subscriptions** — real-time event streaming for live PR/issue updates
- **Full GitHub App permissions** — per-installation permission scoping (read/write per resource type)
- **Webhook delivery UI** — web dashboard for inspecting webhook deliveries
- **Cost controls** — per-pool spending limits, cloud cost tracking, auto-shutdown on budget
