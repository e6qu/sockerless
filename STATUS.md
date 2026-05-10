# Sockerless — Status

Roadmap [PLAN.md](PLAN.md) · resume [DO_NEXT.md](DO_NEXT.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Snapshot

| | |
|---|---|
| Active branch | `state-save-post-pr139` — PR #140 awaiting CI / review / merge. |
| In-flight | Phase 81 (per-instance logs + live console) + Phase 82 (cloud-resources rollup) shipped on PR #140 alongside the post-#139 state save. |
| Last merged | PR #139 — Phase 80 admin UI Topology page (2026-05-10). |
| Cells | 8/8 runner-integration cells GREEN since 2026-05-07. |
| Bugs | 0 open. |
| Live infra | None up. |

**Invariant:** components stay decoupled from admin / UI. Sims, backends, bleephub run independently via env vars; admin only reads what they already expose (`/v1/health`, `/v1/info`). Phase 81 SSE tails admin's own `.stack-pids/<name>.log`; Phase 82 rollup queries existing `/internal/v1/resources` endpoints — no new component-side wiring.

## Next branch — Phase 83 (sim UI parity)

After #140 merges, start a new branch. Goal: lift `ui/packages/sim-{aws,gcp,azure}` onto the same `BackendApp` shell shape backend UIs use, then retire the legacy `/ui/resources` and `/ui/projects/:name/logs` pages.

Concrete deliverables:

1. Containers / Resources / Metrics pages on each sim, mounted via the shared `BackendApp` shell (kicker / nav / dark mode / toast).
2. `ToastProvider` + `ErrorBoundary` at each sim's app root.
3. `ThemeToggle` in the AppShell nav.
4. Reuse Phase 81 infrastructure: SSE log tail + API console panel work for sims unchanged because both endpoints key on topology-instance name, not kind.
5. Retire `/ui/resources` (legacy registry-backed) once `/ui/topology/resources` covers the same use cases. `/ui/projects/:name/logs` similarly retires once topology-driven SSE is wired into project pages.

Sim package sizes today: sim-aws 247 LOC, sim-gcp 228 LOC, sim-azure 221 LOC, frontend-docker 185 LOC vs admin 5.4k. Net code should drop after the consolidation, not grow.

## Phases after 83

- **Phase 84** — per-instance state isolation. `SIM_STATE_DIR=…/.sockerless-state/<project>/<instance>/`; sims gain optional persistent state across restarts, multiple sim instances of the same cloud coexist.
- **Phase 85** — config edit + hot reload. Admin annotates config keys hot-reloadable vs restart-required; UI writes back to `sockerless.yaml` and triggers reload or restart accordingly.
- **Phase 86** — health + supervision surface. Mark unhealthy on process exit / non-2xx `/v1/health` / 5 s timeout; show last-N log lines + diagnostic links. No auto-restart.
- **Phase 87** — centralized observability (Stack A: OTel Collector + VictoriaLogs + Jaeger). Plan in PLAN.md; lands after 86 because 86's "show last-N log lines on unhealthy" is the file-tail source that 87 promotes to OTel.

After 87: phases 91–94 (real per-cloud volume provisioning), live-cloud validation track (Lambda / Cloud Run Services / ACA Apps / AZF cloud-dns / Lambda service-mesh / ACA-AZF Azure AD).

## Recently shipped

| Date | PR | Headline |
|---|---|---|
| 2026-05-10 | #140 (open) | Phase 81 + Phase 82 — SSE log endpoint + single-instance tail UI + instance proxy endpoint + combined timeline + API console UI; cloud-resources rollup endpoint + UI with instance/cloud/service/flat groupings + failed-sources banner. |
| 2026-05-10 | #139 | Phase 80 — admin UI Topology page (`/ui/topology`): project + instance tree, per-instance status, Start/Stop/Rebuild, port registry. |
| 2026-05-10 | #138 | Phase 79 — `sockerless.yaml` topology store, `TopologyManager`, CRUD REST surface, `make/components.mk` lifecycle targets, port allocator. + Phase 87 plan + `specs/CLOUD_RESOURCE_MAPPING.md` consolidation. |
| 2026-05-10 | #137 | Phase 78 UI polish (dark mode, Toast/InlineError, Modal, a11y, perf, READMEs) + Phase 79 step 1 (`Instance` type). |
| 2026-05-10 | #136 | Phase 121b finish — driver consolidation, host-aliases everywhere, AZF/Lambda DNS, Azure AD access. |
| 2026-05-09 | #135 | Phase 121b initial — Azure sim hardening, all-6-backends harness restructure, driver consolidation, GCP sim Cloud Run routing, envelope parsing, label round-trip. |

Older PRs in [WHAT_WE_DID.md](WHAT_WE_DID.md).
