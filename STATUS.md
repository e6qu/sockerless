# Sockerless — Status

Roadmap [PLAN.md](PLAN.md) · resume [DO_NEXT.md](DO_NEXT.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · vibe catalogue [docs/VIBE_CODING.md](docs/VIBE_CODING.md).

## Snapshot

| | |
|---|---|
| Active branch | `phase-162-vibe-slop-catalogue-refresh` — to open as PR #162. |
| In-flight | **Phase 162 — vibe-coding anti-pattern catalogue refresh.** Doc-only follow-on to PR #161 (merged). 12 new patterns (24–35) in `docs/VIBE_CODING.md` from Phase 161 fix lessons + late-2025/2026 external research; `avoid-vibe-slop` skill expanded from 17 to 26 checklist items. No code-surface changes. |
| Last merged | PR #161 — Phase 161 vibe-slop sweep + 18 BUGs closed + bleephub GraphQL completion (2026-05-16, merged at `841f2456`). |
| Standing merge auth | **None.** Default "never auto-merge" rule active. User merges every PR. |
| Cells | 8/8 runner-integration cells GREEN since 2026-05-07. |
| Bugs | 0 open · 1012 fixed (1012 total filed) · 2 false positives. |
| Live infra | None up. |

## Invariants (carry across compactions / fresh sessions)

### Process

- **Never auto-merge PRs.** Push, wait for `gh pr checks` green, ping user. One-time exceptions don't carry forward.
- **Single-branch rule.** All in-flight work for one phase lands on one branch; many granular commits, one PR.
- **State save every task.** STATUS.md + DO_NEXT.md + WHAT_WE_DID.md + MEMORY.md + `_tasks/done/`.
- **Test all the time.** `go test ./...` in every touched module; harness-touch re-runs the harness; terraform-touch runs `terragrunt validate`.
- **Branch hygiene.** Rebase phase branch on `origin/main` before pushing; sync local `main` after merge.
- **Pre-push hooks own the truth.** If `check-latest-deps` flags dep drift, bump deps in the same branch — never skip the hook.
- **Read `.claude/skills/avoid-vibe-slop/SKILL.md` before every non-trivial change** — the catalogue this sweep is closing exists to be applied at write-time.

### Architecture

- **Components stay decoupled from admin / UI.** Sims, backends, bleephub run independently via env vars; admin reads only `/v1/health`, `/v1/info`, env. No admin-required env vars on components, no startup registration.
- **Backend ↔ host primitive must match.** ECS in ECS, Lambda in Lambda, Cloud Run in Cloud Run, GCF in CRF, ACA in ACA, AZF in AZF.
- **No fakes / no fallbacks.** Unknown values fail loud. Operator-requested persistence + auth never silently degrade.
- **Persistence is opt-in + fail-loud.** `BLEEPHUB_PERSIST=true` / `SIM_PERSIST=true` → SQLite. Open-failure *and* write-failure must surface (BUG-985/986 + BUG-997); never silent in-memory fallback.
- **HTTP handlers in `backends/core/handle_*.go` must dispatch through `s.self.<Method>`** — never read `s.Store.*` directly. BUG-991/992 + BUG-995 lineage.
- **Test target gating.** Backend integration tests require `SOCKERLESS_TEST_TARGET=sim|cloud`; never implicit skip.
- **No phase or bug IDs in code comments** (BUG-994 sweep). Metadata lives in commits / PRs / BUGS.md; comments document the *why*.
- **specs/CLOUD_RESOURCE_MAPPING.md is authoritative** for "how does sockerless model X on cloud Y."

### bleephub-specific

- **`gh` CLI is the reference adaptor.** If it works against `api.github.com`, it must work against bleephub. No `gh api $URL -f key=val` URL hackery for happy paths.
- **`gh` is HTTPS-only against non-`github.com` hosts.** Quick-start in `bleephub/README.md` covers the self-signed-cert + system-trust path.
- **GitHub Apps and OAuth Apps are separate concepts.** Distinct store entries, distinct token prefixes (`ghp_`/`gho_`/`ghu_`/`ghs_`/`ghr_`).
- **Installation tokens are immutable snapshots.** Re-mint to pick up perm changes.
- **Body coercion is per-GitHub-spec.** `flexBool` / `flexInt` / `flexInt64` / `flexIntSlice` accept both typed and string-coerced JSON (what `gh api -f` sends). Not a fallback; this is the GitHub Rails-layer behavior made explicit.
- **No `alg:none` JWTs in OAuth issuance** — BUG-1000. The token endpoint must verify the client-assertion JWT signature against the App's public key, per GitHub's `/login/oauth/access_token` contract.

## Phase 162 — vibe-coding catalogue refresh (in flight)

Doc-only follow-on to PR #161. Phase 161 fix work surfaced enough recurring shapes — and external sources published enough new analyses between Phase 158 (catalogue's last update) and now — to warrant a major catalogue refresh. Twelve new patterns appended (24–35) with verbatim source quotes + Phase 161 sub-task mappings. `avoid-vibe-slop` skill grew from 17 to 26 checklist items.

## Recently closed phases

| PR | Phase | Headline |
|---|---|---|
| #161 | 161 | Comprehensive vibe-slop sweep + 18 BUGs closed (994–1011 minus 1010 false-positive) + bleephub GraphQL completion (PR.comments, reviewThreads, ProjectV2 with fields, edit history, minimization, issue/PR locking, PR.milestone, real `gh` CLI smoke tests) + ProjectManager instance-based lifecycle rewrite. All 11 CI checks green; merged at `841f2456`. |
| #160 | 160 | Project-local Claude skills (`sim-handler-checklist`, `cross-resource-stack-test`) + `adaptor-fidelity-check` refinement + complete component-README adaptor-led sweep across 6 cloud backends + 2 simulators + bleephub + `cmd/sockerless` + new `cmd/sockerless-admin/README.md` + rewritten `simulators/README.md`. Phase 157 Track A closed. |
| #159 | 159 | AWS sim CloudFront + ACM + Route 53 + WAFv2 + Amplify + IAM SLR/OIDC (11 sub-tasks, `TestStackProductionShape` cross-resource invariants). |
| #158 | 158 | BUG-991 + BUG-992 fixes (handler→`s.self` delegation); `docs/VIBE_CODING.md` 23-pattern catalogue; `docs/GOLANG_STRONG_TYPING.md`; first 3 project-local Claude skills. |
| #157 | 157 | Component ⇄ reference-adaptor docs sweep started; `backends/docker/README.md` rewrite. |
| #155–156 | 155–156 | bleephub + project-wide docs refresh; GCP dep bump. |
| #153–154 | 153–154 | bleephub ↔ GitHub API parity + broad GitHub API sweep (Reactions, Releases, Deployments, PR review threads, Checks, Actions OIDC + JWKS, Pages, branch protection). |

Older PRs (#112–#152) headline-summarised in [PLAN.md § Closed phases](PLAN.md). Narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md).
