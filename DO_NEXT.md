# Do Next

Resume pointer. Updated after every task. Roadmap detail lives in [PLAN.md](PLAN.md); narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md); bug log in [BUGS.md](BUGS.md); runner wiring in [docs/RUNNERS.md](docs/RUNNERS.md).

## Branch state

- `main` synced with `origin/main` at PR #121 merge.
- `origin-gitlab/main` mirrors `origin/main` (in sync as of 2026-04-27 — pre-push hook now mirror-aware via `PRE_COMMIT_REMOTE_NAME`).
- **`phase-110-runner-integration`** — open as the active branch. Phase 110 = real GitHub Actions + GitLab Runner integration against ECS + Lambda backends, plus a live-AWS manual test pass to feed into the harness.

## Up next on this branch (Phase 110 execution order)

1. **State-doc compression.** Compact BUGS / STATUS / WHAT_WE_DID into the new shape: BUGS.md gets 3 sections (Open / False positives / Resolved); state docs keep the last phase or two narratively and collapse older detail. **(in progress)**
2. **Phase 110 plan in PLAN.md.** Document the 4-cell matrix (GH+GL × ECS+Lambda), the local-laptop-→-remote-cloud architecture, the token strategy, and the manual-test 2-h time-box.
3. **Live AWS manual test pass.** Provision live ECS + Lambda per [`manual-tests/01-infrastructure.md`](manual-tests/01-infrastructure.md); walk every track in [`manual-tests/02-aws-runbook.md`](manual-tests/02-aws-runbook.md). 2-hour time-box. **Fix-as-you-go** discipline: record bug to BUGS.md → fix → re-test → continue (per user override of the standing batch-fix rule for this session).
4. **One-time PAT keychain setup (operator).** `gh auth login` (already done) + `security add-generic-password -U -s sockerless-gl-pat -a "$USER" -w` (paste GitLab PAT with `create_runner` + `api` scopes). Token strategy is documented in [`docs/RUNNERS.md`](docs/RUNNERS.md).
5. **4-cell runner harness** end-to-end:
   - GitHub `actions/runner` × ECS — `tcp://localhost:3375`, label `sockerless-ecs`
   - GitHub `actions/runner` × Lambda — `tcp://localhost:3376`, label `sockerless-lambda`
   - `gitlab-runner` × ECS — same daemon as GH ECS runner
   - `gitlab-runner` × Lambda — same daemon as GH Lambda runner
6. **Tear down live AWS.** `terragrunt destroy` per backend env (self-sufficient via `null_resource sockerless_runtime_sweep`).

## Operational state

- **AWS creds: ACTIVE** for this session (operator confirmed 2026-04-27).
- **GitHub PAT:** stored via `gh` keychain (one-time `gh auth login` complete).
- **GitLab PAT:** keychain entry to be created (`security add-generic-password -U -s sockerless-gl-pat …`) — operator's one-time setup, agent cannot do it (interactive password prompt).
- **Runner registration tokens:** ephemeral; minted per harness run, deleted on exit. Never on disk.
- **Pre-push hooks** (check-rebased + update-readme-badges) recognize mirror remotes via `PRE_COMMIT_REMOTE_NAME`; `git push origin-gitlab main` is a clean fast-forward with hooks intact.

## Cross-links

- Roadmap: [PLAN.md](PLAN.md)
- Phase roll-up: [STATUS.md](STATUS.md)
- Narrative: [WHAT_WE_DID.md](WHAT_WE_DID.md)
- Bug log: [BUGS.md](BUGS.md)
- Runner wiring: [docs/RUNNERS.md](docs/RUNNERS.md)
- Architecture: [specs/SOCKERLESS_SPEC.md](specs/SOCKERLESS_SPEC.md), [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md), [specs/BACKEND_STATE.md](specs/BACKEND_STATE.md), [specs/SIM_PARITY_MATRIX.md](specs/SIM_PARITY_MATRIX.md)
- Manual-test runbooks: [manual-tests/](manual-tests/)

## Standing rules (carry forward)

- **No fakes, no fallbacks, no workarounds** — every gap is a real bug, every bug ships a real fix in the same session.
- **Sim parity per commit** — any new SDK call added to a backend must update [specs/SIM_PARITY_MATRIX.md](specs/SIM_PARITY_MATRIX.md) and add the sim handler in the same commit.
- **State save after every task** — PLAN / STATUS / WHAT_WE_DID / DO_NEXT / BUGS / memory.
- **Never merge PRs** — user handles all merges; agent only creates PRs and waits for CI.
- **Branch hygiene** — rebase PR branch on `origin/main` before push; sync local `main` after push.
