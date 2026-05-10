# Sockerless — Status

Roadmap [PLAN.md](PLAN.md) · resume [DO_NEXT.md](DO_NEXT.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Snapshot

| | |
|---|---|
| Active branch | `phase-84-instance-state-isolation` — Phase 84 work in flight (3 commits + state save). |
| In-flight | Phase 84 — per-instance sim state isolation. BUG-985 fix (sim NewServer fails loud on persistence open) + admin `SIM_DATA_DIR` injection per topology instance + cross-cloud isolation tests. |
| Last merged | PR #141 — Phase 83 sim UI parity (2026-05-10). |
| Cells | 8/8 runner-integration cells GREEN since 2026-05-07. |
| Bugs | 0 open · 985 fixed (BUG-985 added in this branch). |
| Live infra | None up. |

**Invariant:** components stay decoupled from admin / UI. Sims, backends, bleephub run independently via env vars; admin only reads what they already expose (`/v1/health`, `/v1/info`). Phase 81 SSE tails admin's own `.stack-pids/<name>.log`; Phase 82 rollup queries existing `/internal/v1/resources` endpoints — no new component-side wiring.

## Phase 84 — in flight on `phase-84-instance-state-isolation`

Three implementation commits + state save:

1. `phase 84 / BUG-985: sim NewServer fails loud on persistence open` — sim shared `NewServer(cfg) *Server` → `(*Server, error)`. The previous code logged a "falling back to in-memory" warning when `SIM_PERSIST=true` and `OpenDB` failed; that masked misconfiguration (bad path, perms, full disk) and caused silent data loss across restarts. Now the error is wrapped and returned; sim main.go calls `log.Fatalf`. Mirrored across simulators/{aws,gcp,azure}/shared/server.go.
2. `phase 84: admin injects SIM_DATA_DIR per topology instance` — `InstanceLifecycle.Start` gains `project string` and writes `SIM_DATA_DIR=<repo>/.sockerless-state/<project>/<instance>/` into `.stack-pids/<n>.env` for `kind=sim`. New `managedEnvFor` + `mergeConfig` helpers; operator-provided Instance.Config wins on conflict. Operator opts into persistence by setting `SIM_PERSIST=true` in instance Config — admin doesn't force it (components-decoupled invariant).
3. `phase 84: multi-instance isolation tests across 3 sims` — 5 test cases in each cloud's `shared/` package: cross-DataDir isolation, persist-survives-reopen, BUG-985 regression guard (NewServer returns error when `mkdir` fails), persist happy path, no-persist path.

## Phases after 84

- **Phase 84** — per-instance state isolation. `SIM_STATE_DIR=…/.sockerless-state/<project>/<instance>/`; sims gain optional persistent state across restarts, multiple sim instances of the same cloud coexist.
- **Phase 85** — config edit + hot reload. Admin annotates config keys hot-reloadable vs restart-required; UI writes back to `sockerless.yaml` and triggers reload or restart accordingly.
- **Phase 86** — health + supervision surface. Mark unhealthy on process exit / non-2xx `/v1/health` / 5 s timeout; show last-N log lines + diagnostic links. No auto-restart.
- **Phase 87** — centralized observability (Stack A: OTel Collector + VictoriaLogs + Jaeger). Plan in PLAN.md; lands after 86 because 86's "show last-N log lines on unhealthy" is the file-tail source that 87 promotes to OTel.

After 87: phases 91–94 (real per-cloud volume provisioning), live-cloud validation track (Lambda / Cloud Run Services / ACA Apps / AZF cloud-dns / Lambda service-mesh / ACA-AZF Azure AD).

## Recently shipped

| Date | PR | Headline |
|---|---|---|
| 2026-05-10 | #141 | Phase 83 — shared `ResourceListPage` in `@sockerless/ui-core`; 13 sim pages refactored across simulator-aws / gcp / azure; legacy `/ui/resources` + `/ui/projects/:name` + `/ui/projects/:name/logs` retired. |
| 2026-05-10 | #140 | Phase 81 + Phase 82 — SSE log endpoint + single-instance tail UI + instance proxy endpoint + combined timeline + API console UI; cloud-resources rollup endpoint + UI with instance/cloud/service/flat groupings + failed-sources banner. |
| 2026-05-10 | #139 | Phase 80 — admin UI Topology page (`/ui/topology`): project + instance tree, per-instance status, Start/Stop/Rebuild, port registry. |
| 2026-05-10 | #138 | Phase 79 — `sockerless.yaml` topology store, `TopologyManager`, CRUD REST surface, `make/components.mk` lifecycle targets, port allocator. + Phase 87 plan + `specs/CLOUD_RESOURCE_MAPPING.md` consolidation. |
| 2026-05-10 | #137 | Phase 78 UI polish (dark mode, Toast/InlineError, Modal, a11y, perf, READMEs) + Phase 79 step 1 (`Instance` type). |
| 2026-05-10 | #136 | Phase 121b finish — driver consolidation, host-aliases everywhere, AZF/Lambda DNS, Azure AD access. |
| 2026-05-09 | #135 | Phase 121b initial — Azure sim hardening, all-6-backends harness restructure, driver consolidation, GCP sim Cloud Run routing, envelope parsing, label round-trip. |

Older PRs in [WHAT_WE_DID.md](WHAT_WE_DID.md).
