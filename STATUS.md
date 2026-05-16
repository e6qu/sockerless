# Sockerless — Status

Roadmap [PLAN.md](PLAN.md) · resume [DO_NEXT.md](DO_NEXT.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · vibe catalogue [docs/VIBE_CODING.md](docs/VIBE_CODING.md).

## Snapshot

| | |
|---|---|
| Active branch | `phase-164-vibe-slop-sweep-2` — PR #164 open. |
| In-flight | **Phase 164 — second vibe-slop sweep + terraform-provider test expansion.** PR #164 contains 13 commits closing 19 BUGs (1014–1032). Three sweep passes (first / re-verification / third-pass-user-requested) plus deeper terraform-provider coverage on GCP (4 → 11 resources across 6 sim slices) and Azure (1 → 5 resources across 5 sim slices). The expansion surfaced + fixed 3 real sim defects: BUG-1029 (missing GCP secret-version state-transition handlers + bare-version GET); BUG-1030 (GCP terraform-tests carried the BUG-993 close-then-bind port race fixed only in sdk-tests during Phase 160); BUG-1031 / 1032 (terraform-test main.tf expansions). |
| Last merged | PR #163 — Phase 163 Makefile legacy alias rip-out + docs sweep (2026-05-16, merged at `d5b9d22a`). |
| Standing merge auth | **None.** Default "never auto-merge" rule active. User merges every PR. |
| Cells | 8/8 runner-integration cells GREEN since 2026-05-07. |
| Bugs | 0 open · 1032 fixed (1032 total filed) · 2 false positives. |
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

## Phase 164 — Second vibe-slop sweep + terraform expansion (in flight)

Phase 161 was the first comprehensive vibe-slop sweep (18 BUGs closed). Phase 164 re-runs the [`avoid-vibe-slop`](../.claude/skills/avoid-vibe-slop/SKILL.md) checklist with fresh eyes after Phase 162 grew `docs/VIBE_CODING.md` from 23 → 35 patterns + expanded the skill from 17 → 26 checklist items. Pattern 26 / 32 (re-verification with fresh eyes) explicitly predicted the first sweep would rubber-stamp some violations — it did. **19 new BUGs closed (1014–1032).**

The phase ran in five layered passes per user direction:

1. **First-pass survey** — 9 BUGs (1014–1022) filed up front in `BUGS.md`; sub-task table in `DO_NEXT.md`.
2. **Severity-ordered fix wave** — P164.1..P164.8 closed each BUG in its own commit. Headlines: stripped `(BUG-944)` literal from a Cloud Functions volume-translator operator-visible error string + rewrote the matching test assertion from the contract; strict-decode on all four bleephub write handlers swallowing malformed JSON; strict-decode sweep across AWS + GCP sims (WAFv2, Amplify, GCP AR/CRJ/GCF); strict-decode on `backends/core` exec + libpod handlers; strict-decode on `cloudrun-functions` cloud-state docker-label JSON; ripped two dead `bleephub/webhooks_payloads.go` helpers + the matching `var _ = json.Marshal` silencer; dropped stale `//nolint:unused` pragmas + the unused `flexInt64` type from `gh_request_decode.go`; swept five more unused-import silencers across `bleephub/` + `simulators/aws/`; finished the BUG-994 phase-ref sweep at 10 production-code sites.
3. **Re-verification pass** (pattern 26 / 32) — 3 further BUGs (1023–1025): the `stringifyJobState` dead helper in github-runner-dispatcher-gcp, the `httputil.DumpRequest` silencer in tools/http-trace, and three silent `pktline.Encoder` Encodef/Flush swallows in bleephub git_http.go (now Debug-level logged).
4. **Third-pass user-requested sweep** — 3 more BUGs (1026–1028): two test files asserting on Phase metadata in error strings (pattern 28), one naked `t.Skip()` with no message, and the Azure terraform-tests docs↔code mismatch (README + apply_test.go said "azurerm" while main.tf used "azurestack").
5. **Terraform-provider test expansion** (user-requested) — 4 more BUGs (1029–1032). GCP terraform-tests expanded from 4 resources (compute_network, compute_disk, public+private DNS zones) to 11 resources covering 6 sim slices: compute, dns, artifactregistry, cloud_run_v2 Service + Job, storage, secretmanager. Azure terraform-tests expanded from 1 (resource_group) to 5 (+ virtual_network, subnet, network_security_group, network_security_rule). Expansion surfaced two real sim defects: missing GCP secret-version state-transition handlers (`:enable`/`:disable`/`:destroy`) + the same close-then-bind port-allocator race in GCP terraform-tests that Phase 160 fixed in sdk-tests. Both closed in the same commit as the test expansion. AWS terraform-tests was already comprehensive (394 lines + cross-resource invariants from Phase 159) — not touched.

PR #164 acceptance: 19 BUGs closed, 0 open, 11 standard CI checks green per push, user merges.

## Phase 163 — Makefile legacy rip-out + docs sweep (merged)

User directive: "sockerless has no legacy, it's under active development." The top-level Makefile carried a `# ── Legacy aliases ──` section that preserved pure-alias targets purely for muscle-memory / pre-Phase-79 CI invocations: `sim-test-{ecs,lambda,cloudrun,gcf,aca,azf,aws,gcp,azure,all}`, `test-{unit,e2e,agent,core,bleephub}`, `bleephub-test`, `bleephub-gh-test`. Every one of those just delegated `$(MAKE) -C <dir> <target>` — which the standard `%/<target>` path-delegation rule already covers. Dropped them; reframed the remaining real recipes (Docker-driven `smoke-test-*`, `tf-int-test-*`, `e2e-github-*`, `e2e-gitlab-*`, `upstream-test-*`, `bleephub-gh-docker-test`) under canonical section headers (no "legacy" framing).

Side-fix surfaced during verification: the `%/test %/test-integration …` pattern rule was being short-circuited when a target like `bleephub/test` happened to collide with a real directory on disk (`bleephub/test/`). Added a `FORCE` phony dep so the recipe always runs.

Docs sweep — replaced every `make sim-test-*`, `make bleephub-test`, `make bleephub-gh-test`, `make test-{unit,e2e,agent,core,bleephub}` reference with its canonical path-delegation form (`make backends/<x>/test-integration`, `make bleephub/test`, `make bleephub/test-integration`, `make tests/test`, `make agent/test`, `make backends/core/test`). Three stale doc refs to make targets that never existed (`stack-aws-ecs-up`, `stack-aws-ecs-down`, `e2e-github-aws-ecs`, `docker-tf-int-test-azure`) replaced with the real names. Stripped "legacy 1-sim + 1-backend tuple" comments from `make/stack.mk` + `make/components.mk` — the pre-canned topology is canonical, not legacy.

## Recently closed phases

| PR | Phase | Headline |
|---|---|---|
| #162 | 162 | Vibe-coding catalogue refresh — 12 new patterns (24–35) in `docs/VIBE_CODING.md` from Phase 161 fix lessons + late-2025/2026 external research; `avoid-vibe-slop` skill expanded from 17 to 26 checklist items. Doc-only. Merged at `4f602988`. |
| #161 | 161 | Comprehensive vibe-slop sweep + 18 BUGs closed (994–1011 minus 1010 false-positive) + bleephub GraphQL completion (PR.comments, reviewThreads, ProjectV2 with fields, edit history, minimization, issue/PR locking, PR.milestone, real `gh` CLI smoke tests) + ProjectManager instance-based lifecycle rewrite. All 11 CI checks green; merged at `841f2456`. |
| #160 | 160 | Project-local Claude skills (`sim-handler-checklist`, `cross-resource-stack-test`) + `adaptor-fidelity-check` refinement + complete component-README adaptor-led sweep across 6 cloud backends + 2 simulators + bleephub + `cmd/sockerless` + new `cmd/sockerless-admin/README.md` + rewritten `simulators/README.md`. Phase 157 Track A closed. |
| #159 | 159 | AWS sim CloudFront + ACM + Route 53 + WAFv2 + Amplify + IAM SLR/OIDC (11 sub-tasks, `TestStackProductionShape` cross-resource invariants). |
| #158 | 158 | BUG-991 + BUG-992 fixes (handler→`s.self` delegation); `docs/VIBE_CODING.md` 23-pattern catalogue; `docs/GOLANG_STRONG_TYPING.md`; first 3 project-local Claude skills. |
| #157 | 157 | Component ⇄ reference-adaptor docs sweep started; `backends/docker/README.md` rewrite. |
| #155–156 | 155–156 | bleephub + project-wide docs refresh; GCP dep bump. |
| #153–154 | 153–154 | bleephub ↔ GitHub API parity + broad GitHub API sweep (Reactions, Releases, Deployments, PR review threads, Checks, Actions OIDC + JWKS, Pages, branch protection). |

Older PRs (#112–#152) headline-summarised in [PLAN.md § Closed phases](PLAN.md). Narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md).
