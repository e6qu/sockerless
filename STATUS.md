# Sockerless — Status

**103 phases closed (Phase 108 closed 2026-04-26). 835 bugs tracked — 835 fixed, 0 open. 1 false positive.** PR #118 merged. PR #120 open with: 22 audit closures (BUG-802 + 638/640/646/648 retro + 804/806 + 820..831 + 832..835); **Phase 104 framework migration complete** — all 13 typed adapters shipped, every dispatch site flowing through TypedDriverSet, framework renamed to drop 104 suffix; Phase 105 waves 1-3 (libpod-shape golden tests, 8 handlers); Phase 108 closed in-branch (77/77 sim-parity matrix ✓ — 33 AWS / 16 GCP / 28 Azure); manual-tests directory + repo-wide code/doc cleanup. **Next on this branch:** per-backend cloud-native typed driver overrides — replace legacy adapter defaults with real typed cloud drivers.

See [PLAN.md](PLAN.md) (roadmap), [BUGS.md](BUGS.md) (bug log), [WHAT_WE_DID.md](WHAT_WE_DID.md) (narrative), [DO_NEXT.md](DO_NEXT.md) (resume pointer).

## Branch state

- **`main`** — synced with `origin/main` at PR #119 merge.
- **`post-pr-118-bug-audit-and-phases`** — open as PR #120, ~20 commits ahead of main.
- **`origin-gitlab/main`** — mirror, lags; pushed when convenient.

## Recent merges

| PR | Summary |
|---|---|
| #120 (open) | Post-PR-#118 audit + Phase 104 lifts 1+2 + Phase 105 waves 1-3 + Phase 108 closed (BUG-802 + 638/640/646/648 retro + 804/806 + 820..831 + 832..835). |
| #119 | Post-PR-#118 state-doc refresh — Phase 104 promoted to active. |
| #118 | Round-8 + Round-9 live-AWS sweep — 30 bugs (BUG-786..819), per-cloud terragrunt sweep parity. |
| #117 | Round-7 live-AWS sweep — 16 bugs (BUG-770..785). |
| #115 | Phases 96/98/98b/99/100/101/102 + 13-bug audit sweep. |
| #114 | Phase 91 ECS EFS volumes + BUG-735/736/737. |

## Open work (full detail in [PLAN.md](PLAN.md))

- **Phase 104** — cross-backend driver framework. **Framework migration complete + first cloud-native overrides shipped.** All 13 adapters; every dispatch site (Exec, Attach, Logs, Signal, ProcList, FSDiff, FSRead, FSWrite, FSExport, Commit, Build, Registry-Pull, Registry-Push) flows through TypedDriverSet. All 6 cloud backends (Lambda, GCF, AZF, ECS, Cloud Run, ACA) now use cloud-native typed Logs + Attach (CloudWatch / Cloud Logging / Azure Monitor) instead of the legacy adapter, bypassing s.self.ContainerLogs / ContainerAttach. Remaining: per-backend cloud-native typed drivers for the other dimensions (Exec, Signal, FS*, Commit, Build, Registry, ProcList) — those are a longer arc since each cloud has a distinct path. Stats has no api.Backend dispatch site (handler inline).
- **Phase 105** — libpod-shape conformance, rolling. Waves 1-3 done; wave 4 (events stream, exec start hijack, container CRUD) lower-priority.
- **Phase 106** — real GitHub Actions runner integration. Architecture sketched (per-backend daemon v1; label-dispatch v2 via Phase 68). ECS + Lambda first.
- **Phase 107** — real GitLab Runner integration (origin-gitlab mirror). Same shape; `dind` sub-test included.
- **Phase 68** — Multi-Tenant Backend Pools. P68-001 done; 9 sub-tasks remaining; Phase 106 label-routing motivates this.
- **Live-cloud runbooks** — GCP + Azure terraform live envs to add; per-cloud `sockerless_runtime_sweep` makes destroy self-sufficient.

## Test counts (head of `main`)

| Category | Count |
|---|---|
| Core unit | 312 |
| Cloud SDK/CLI | AWS 68, GCP 64, Azure 57 |
| Sim-backend integration | 77 |
| GitHub E2E | 186 |
| GitLab E2E | 132 |
| Terraform | 75 |
| UI/Admin/bleephub | 512 |
| Lint (18 modules) | 0 |

## AWS access key state

Root-account access key `AKIA2TQEGRDBRV2KFW6L` deactivated by maintainer 2026-04-26 post-round-9. **Reactivate via AWS Console before any future live-AWS test pass** (Phase 106 ECS workloads, Phase 87 live-GCP doesn't need it, Phase 88 live-Azure doesn't need it). Per-cloud `null_resource sockerless_runtime_sweep` (BUG-819) makes `terragrunt apply` + `terragrunt destroy` self-sufficient.
