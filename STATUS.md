# Sockerless — Status

**104 phases closed (Phase 109 closed in PR #121, merged 2026-04-27). 844 bugs tracked — 844 fixed, 0 open. 1 false positive.** Active branch: **`phase-110-runner-integration`** (Phase 110 = real GitHub + GitLab runner integration against ECS + Lambda backends, plus a live-AWS manual test pass — see [PLAN.md § Phase 110](PLAN.md) and [docs/RUNNERS.md](docs/RUNNERS.md)). PRs #117 / #118 / #119 / #120 / #121 merged. Mirror `origin-gitlab/main` in sync with `origin/main` as of 2026-04-27.

See [PLAN.md](PLAN.md) (roadmap), [BUGS.md](BUGS.md) (bug log), [WHAT_WE_DID.md](WHAT_WE_DID.md) (narrative), [DO_NEXT.md](DO_NEXT.md) (resume pointer), [docs/RUNNERS.md](docs/RUNNERS.md) (runner wiring).

## Branch state

- **`main`** — synced with `origin/main` at PR #121 merge.
- **`origin-gitlab/main`** — mirror, pre-push hooks now mirror-aware so `git push origin-gitlab main` is a clean fast-forward.
- **`phase-110-runner-integration`** — active; baseline commit `f5c1ab7` shipped `docs/RUNNERS.md` + the mirror-aware hooks.

## Recent merges

| PR | Summary |
|---|---|
| #121 | Phase 109 strict cloud-API fidelity sweep — 19 audit items: AWS Lambda VpcConfig + region/account scoping + Secrets Manager + SSM Parameter Store + KMS + DynamoDB; GCP `compute.firewalls` + `compute.routers`/Cloud NAT + `iam.generateAccessToken` + operations endpoint persistence; Azure IMDS token endpoint + Blob Container ARM CRUD + NSG priority+direction validation + Private DNS AAAA/CNAME/MX/PTR/SRV/TXT records + NAT Gateways + Route Tables + Container Apps/Jobs Azure-AsyncOperation polling + Key Vault ARM+data plane + ARM `SystemData.createdAt` preservation. Test-fixtures no-fakes audit clean. |
| #120 | Audit + Phase 104 framework migration + cloud-native typed drivers + Phase 105 waves 1-3 + Phase 108 closed + Phase 106/107 harness scaffolding + ImageRef domain type + Phase 109 first-round (BUG-836..844: real ECS lifecycle, real SSM AgentMessage protocol, real subnet-CIDR IP allocation, real Azure per-site hostnames, real kill signal routing) + repo-wide code/doc cleanup. |
| #119 | Post-PR-#118 state-doc refresh — Phase 104 promoted to active. |
| #118 | Round-8 + Round-9 live-AWS sweep — 30 bugs (BUG-786..819), per-cloud terragrunt sweep parity. |
| #117 | Round-7 live-AWS sweep — 16 bugs (BUG-770..785). |

Older PRs (#112–#115) — sim parity, real volumes, FaaS invocation tracking, reverse-agent ops, Phase 91/86/87/88 closures. Detail in [WHAT_WE_DID.md](WHAT_WE_DID.md) and per-bug entries in [BUGS.md](BUGS.md).

## Open work

- **Phase 110** — real GitHub + GitLab runner integration. 4-cell matrix (GH + GL × ECS + Lambda); local Sockerless daemons (`:3375`/`:3376`) dispatching to live AWS; ephemeral runner-registration tokens minted per harness run. Live-AWS manual test pass (2-h time-box) seeds the harness with a known-good baseline. Architecture + token strategy: [docs/RUNNERS.md](docs/RUNNERS.md). Operator one-time setup: `gh auth login` (done) + `security add-generic-password -U -s sockerless-gl-pat -a "$USER" -w` for the GitLab PAT.
- **Phase 104 wrapper-removal pass** — gated on docker getting typed cloud-native drivers OR accepting wrappers as permanent. Once decided: drop unused `WrapLegacyXxx` / `LegacyXxxFn` scaffolding and shrink `api.Backend` correspondingly. Coordinated landing.
- **Phase 104 interface tightening** — typed `Signal` enum, `ResolveImageReg(ImageRef)` helper, structured `Stats` struct.
- **Phase 105 wave 4** (lower priority) — events stream, exec start hijack shape, container CRUD beyond list.
- **Phase 68** (Multi-Tenant Backend Pools) — P68-001 done; 9 sub-tasks remain. Phase 110's 4-cell setup uses Phase-68-v1 (one daemon per backend); Phase 68-v2 collapses to label-based dispatch on a single daemon.

## Test counts (head of `main`)

| Category | Count |
|---|---|
| Core unit | 312 |
| Cloud SDK/CLI | AWS 68+, GCP 64+, Azure 57+ |
| Sim-backend integration | 77 (parity matrix at 77/77 ✓) |
| Libpod golden-shape | 8 |
| External-suite replays | 12 (act + gitlab-ci-local) |

## Operational state

- **AWS creds: ACTIVE** for the Phase 110 manual-test session (2026-04-27).
- **Live AWS infra** — torn down post-PR #118; per-cloud `null_resource sockerless_runtime_sweep` (BUG-819) means re-apply + destroy are self-sufficient.
- **PAT keychain entries** — `gh` (GitHub) keychain-backed; GitLab PAT to be added by operator via `security(1)` (one-time, interactive — agent cannot do it).
