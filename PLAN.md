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

## Phase 86 — Complete Runner Support (ECS + Lambda × github.com + gitlab.com SaaS)

Close the gap between "Docker API passes E2E in simulator mode" and "official `actions/runner` + `gitlab-runner` binaries run real CI jobs on real AWS against real github.com / gitlab.com." No `-wasm` / synthetic shortcuts: services, custom images, and docker build must work for real. Lambda 15-min cap accepted.

Work partitioned into **no-AWS-credentials** (can be done now, verified in simulator) and **needs-AWS** (verified against live accounts).

### No-AWS track

| Task | Status | Description |
|---|---|---|
| P86-001 | done | Dropped `-wasm` / `-faas` variant-routing. `get_test_variant` and `should_skip_for_faas` removed from `tests/e2e-live-tests/lib.sh`; orchestrators use test names directly |
| P86-002 | done | Pruned `services` / `custom-image` from `ALL_WORKFLOWS` / `ALL_PIPELINES` (files removed in daeff00). Renamed `container-action-faas.yml` → `container-action.yml`. `docs/runner-capability-matrix.md` added as TBD template |
| P86-003 | done | Per-hostname Cloud Map services + `DnsSearchDomains` on task def. Old shared `containers` service removed. Unit tests + `docs/ECS_SERVICES_DESIGN.md`. Full DNS end-to-end verification belongs to the live-AWS track |
| P86-004 | in-progress | Lambda `ContainerStop` / `ContainerKill` clamp function timeout via `UpdateFunctionConfiguration(Timeout=1)` + disconnect reverse agent (stub pending P86-005) + close wait channel. `docker logs -f` follow mode pending |
| P86-005 | | Lambda live-mode exec design: reverse-agent as Lambda handler, callback URL plumbing (requires code + config changes, no live creds needed to implement) |
| P86-006 | | Add `--runner official` switch to E2E harnesses to target unmodified `actions/runner` and `gitlab-runner` binaries (vs `act` / self-hosted GitLab CE) |
| P86-007 | | Docs: `docs/ECS_LIVE_SETUP.md`, `docs/GITHUB_RUNNER_SAAS.md`, `docs/GITLAB_RUNNER_SAAS.md` — provisioning, TLS/auth, runner registration |
| P86-008 | | Unit + integration tests for P86-003/004/005 |

### Needs-AWS track (blocked on credentials)

| Task | Status | Description |
|---|---|---|
| P86-009 | | Provision reference AWS environment (VPC, IAM, ECR, CloudWatch, optional EFS); sanity-check live ECS Docker CLI regression still passes |
| P86-010 | | `actions/runner` × real github.com × live ECS — three job shapes: plain shell, `container:`, `services:` |
| P86-011 | | `gitlab-runner` × real gitlab.com × live ECS — same matrix |
| P86-012 | | Lambda live Docker CLI baseline (Round 1 equivalent to ECS Round 1) |
| P86-013 | | Lambda live runner profile: viability decision (restricted profile or drop from runner scope); land outcome in docs |
| P86-014 | | Save final state |

---

## Future Ideas

- GraphQL subscriptions for real-time event streaming
- Full GitHub App permission scoping
- Webhook delivery UI
- Cost controls (per-pool spending limits, auto-shutdown)
