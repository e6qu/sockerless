# Sockerless — Status

Roadmap [PLAN.md](PLAN.md) · resume [DO_NEXT.md](DO_NEXT.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Snapshot

| | |
|---|---|
| Active branch | `phase-158-bug991-vibecoding-skills` — off `origin/main` post-PR-#157 merge. |
| In-flight | Phase 158 — BUG-991 fix + `docs/VIBE_CODING.md` sourced catalogue + 3 project-local Claude skills (`avoid-vibe-slop`, `adaptor-fidelity-check`, `manual-test`). |
| Last merged | PR #157 — Phase 157 component ⇄ reference-adaptor docs sweep (2026-05-13). |
| Standing merge auth | **Expired.** Default "never auto-merge" rule active. User merges every PR. |
| Cells | 8/8 runner-integration cells GREEN since 2026-05-07. |
| Bugs | 1 open (BUG-992 staged as Phase 159) · 991 fixed (BUG-991 closed this phase). |
| Live infra | None up. |

## Invariants (carry across compactions / fresh sessions)

### Process

- **Never auto-merge PRs.** Push, wait for `gh pr checks` green, ping user. One-time exceptions don't carry forward.
- **Single-branch rule.** All in-flight work for one phase lands on one branch; many granular commits, one PR.
- **State save every task.** STATUS.md + DO_NEXT.md + WHAT_WE_DID.md + MEMORY.md + this file's `_tasks/done/`.
- **Test all the time.** `go test ./...` in every touched module; harness-touch re-runs the harness; terraform-touch runs `terragrunt validate`.
- **Branch hygiene.** Rebase phase branch on `origin/main` before pushing; sync local `main` after merge.
- **Pre-push hooks own the truth.** If the `check-latest-deps` hook flags dep drift, bump deps in the same branch — never skip the hook.

### Architecture

- **Components stay decoupled from admin / UI.** Sims, backends, bleephub run independently via env vars; admin reads only `/v1/health`, `/v1/info`, env. No admin-required env vars on components, no startup registration.
- **Backend ↔ host primitive must match.** ECS in ECS, Lambda in Lambda, Cloud Run in Cloud Run, GCF in CRF, ACA in ACA, AZF in AZF.
- **No fakes / no fallbacks.** Unknown values fail loud. Operator-requested persistence + auth never silently degrade.
- **Persistence is opt-in + fail-loud.** `BLEEPHUB_PERSIST=true` / `SIM_PERSIST=true` → SQLite. Open-failure `log.Fatalf` (BUG-985/986 pattern); never silent in-memory fallback.
- **Test target gating.** Backend integration tests require `SOCKERLESS_TEST_TARGET=sim|cloud`; never implicit skip.
- **specs/CLOUD_RESOURCE_MAPPING.md is authoritative** for "how does sockerless model X on cloud Y."

### bleephub-specific (closed in Phases 153–156)

- **`gh` CLI is the reference adaptor.** If it works against `api.github.com`, it must work against bleephub. Test harness uses native `gh repo create / view / list`, `gh issue create / view / list / close / reopen`, `gh pr create / view / list / review / merge`, `gh release create`. No `gh api` URL hackery for happy path.
- **`gh` is HTTPS-only against non-`github.com` hosts.** Quick-start in `bleephub/README.md` covers the self-signed-cert + system-trust path; `host:port` in `--hostname` works if you can't bind `:443`.
- **GitHub Apps and OAuth Apps are separate concepts.** Distinct store entries, distinct token prefixes (`ghp_`/`gho_`/`ghu_`/`ghs_`/`ghr_`/`bph_`).
- **Installation tokens are immutable snapshots.** Re-mint to pick up perm changes.
- **Body coercion is per-GitHub-spec.** `flexBool` / `flexInt` / `flexInt64` / `flexIntSlice` accept both typed and string-coerced JSON (what `gh api -f` sends). Not a fallback; this is the GitHub Rails-layer behavior made explicit.

## Phase 158 — BUG-991 + VIBE_CODING.md + Claude skills (in flight)

Three pieces on one branch:

1. **BUG-991 fix** ✅ — `handleContainerWait` non-CloudState branch + `BaseServer.ContainerWait` `condition=removed` fallback replaced with `s.self.ContainerInspect` + `s.self.ContainerWait` delegation. Verified: `docker run --rm alpine:3.20 echo hi` succeeds against `backends/docker`. Closed the silent-success-on-missing-resource fallback per "no fallback-hiding-bugs."
2. **`docs/VIBE_CODING.md`** ✅ — 23-pattern sourced catalogue with verbatim quotes + URLs (HN, Addy Osmani, Simon Willison, Augment, curl, Zig, TDS, Socket, CACM). Each pattern maps to a sockerless-specific failure mode + policy + bug-ID where applicable.
3. **`.claude/skills/{avoid-vibe-slop,adaptor-fidelity-check,manual-test}/SKILL.md`** ✅ — three project-local Claude skills operationalising the catalogue. Skeptical-of-imports: all three authored from scratch, no external skill imports (avoiding supply-chain / backdoor risk).

Sub-bug surfaced (BUG-992): `docker images` / `docker volume ls` / `docker network ls` return empty against passthrough backends because list handlers read `s.Store.X.List()` directly. Same shape as BUG-991. Staged as Phase 159 in [PLAN.md](PLAN.md).

Full plan in [PLAN.md § Phase 158](PLAN.md). Component-adaptor matrix from Phase 157 in [DO_NEXT.md](DO_NEXT.md).

## Recently closed phases

| PR | Phase | Headline |
|---|---|---|
| #157 | 157 | Component ⇄ reference-adaptor docs sweep + state save + experimental/security caveat on root README + `backends/docker/README.md` rewrite. BUG-991 surfaced. |
| #156 | 156 | Project-wide docs refresh + bleephub Quick start + `gh` CLI `--hostname` clarification + GCP dep bump. |
| #155 | 155 | bleephub-specific docs refresh — `bleephub/README.md`, `docs/BLEEPHUB_GH_CLI.md`, `specs/BLEEPHUB_GITHUB_API_PARITY.md`, `ARCHITECTURE.md`. |
| #154 | 154 | Broad GitHub API sweep — reactions, releases, deployments + environments, PR review comments + threads, Checks, Actions OIDC + JWKS, Pages, branch protection. Real `gh` CLI Docker harness 50/50 PASS. |
| #153 | 153 | bleephub ↔ GitHub API parity + SQLite persistence + real `gh` CLI compat (13 sub-tasks). |
| #152 | docs | `docs/POD_MATERIALIZATION.md` — per-backend pod materialization walked through GH + GitLab runners. |
| #151 | 87d + 92 | Trace propagation + MeterProvider + runtime metrics + `make stack-observability-validate`; `Backing: gcs-fuse` deregistered on cloudrun + gcf. |
| #150 | 87c | zerolog → OTel logs bridge across all 12 components. |
| #149 | 91 | Lambda volume_translator framework migration; cloudrun + gcf reject `BackingPDEphemeral`. |

Older PRs (#112–#148) headline-summarised in [PLAN.md § Closed phases](PLAN.md). Narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md).
