# Sockerless — Status

Roadmap [PLAN.md](PLAN.md) · resume [DO_NEXT.md](DO_NEXT.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · vibe catalogue [docs/VIBE_CODING.md](docs/VIBE_CODING.md).

## Snapshot

| | |
|---|---|
| Active branch | `phase-164-vibe-slop-sweep-2` — to open as PR #164. |
| In-flight | **Phase 164 — second vibe-slop sweep.** Re-running the [`avoid-vibe-slop`](../.claude/skills/avoid-vibe-slop/SKILL.md) checklist with fresh eyes against `origin/main` at `d5b9d22a` after `docs/VIBE_CODING.md` grew to 35 patterns in Phase 162. First-pass survey filed 9 new BUGs (1014–1022). Headline shapes: BUG-994 phase-ref sweep incomplete (still in `simulators/aws/ecs.go`, `bleephub/persistence.go`, `backends/lambda/cloud_state.go`, etc., plus `(BUG-944)` literally embedded in a Cloud Functions volume-translator operator-visible error string with a test asserting on the substring); BUG-996 cross-cloud silent-decode sibling in bleephub handlers + AWS/GCP sims + `backends/core` exec & libpod handlers; dead helpers in `bleephub/webhooks_payloads.go` with zero callers but `//nolint:unused // callers land in subsequent commits` directives that never landed; stale `//nolint:unused` pragmas on context helpers that now have real callers; unused-import silencers (`var _ = json.Marshal`). |
| Last merged | PR #163 — Phase 163 Makefile legacy alias rip-out + docs sweep (2026-05-16, merged at `d5b9d22a`). |
| Standing merge auth | **None.** Default "never auto-merge" rule active. User merges every PR. |
| Cells | 8/8 runner-integration cells GREEN since 2026-05-07. |
| Bugs | 9 open · 1013 fixed (1022 total filed) · 2 false positives. |
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

## Phase 164 — Second vibe-slop sweep (in flight)

Phase 161 was the first comprehensive vibe-slop sweep (18 BUGs closed). After Phase 162 grew `docs/VIBE_CODING.md` from 23 → 35 patterns + expanded the `avoid-vibe-slop` skill from 17 → 26 checklist items, the catalogue surfaces a different shape of fault — sycophancy, comprehension debt, expansion-without-pruning, docs-vs-code drift, pre-commit hook rollback, language-aware-tooling-not-sed. Pattern 26 / 32 (re-verification with fresh eyes) explicitly predicted the first sweep would rubber-stamp some violations.

First-pass survey output (9 BUGs in `BUGS.md § Open`):

- **BUG-1014** — Phase / sub-phase refs in production code comments survived the BUG-994 sweep at ~10 sites.
- **BUG-1015** — `backends/cloudrun-functions/volume_translator.go:95` embeds `(BUG-944)` in an operator-visible error string; `volume_translator_test.go:78` asserts on the literal `"BUG-944"` substring — classic pattern-28 anti-pattern.
- **BUG-1016** — bleephub HTTP handlers silently swallow malformed JSON: OIDC custom sub PUT, Pages create, branch protection PUT, issue lock.
- **BUG-1017** — Sim handlers silently swallow JSON decode: WAFv2 UpdateRuleGroup, Amplify StartJob, GCP AR proxy manifest parse, GCF entrypoint resolution, cloudrunjobs Operation marshal-back.
- **BUG-1018** — Core HTTP handlers silently swallow request decode: `handleExecStart` (hijack-after-decode race), `handleLibpodContainerList` (podman specgen shim).
- **BUG-1019** — `backends/cloudrun-functions/cloud_state.go:506` silently decodes Cloud Run docker labels JSON → ghost containers with empty labels on malformed state.
- **BUG-1020** — `bleephub/webhooks_payloads.go` has two helpers with zero callers carrying `//nolint:unused // callers land in the workflow-trigger commit`; the commit never landed.
- **BUG-1021** — Stale `//nolint:unused` pragmas on `gh_middleware.go` context helpers that now have real callers.
- **BUG-1022** — Unused-import silencers (`var _ = json.Marshal` etc.) scattered across bleephub + AWS sim — refusal to delete (pattern 27).

Sub-task ordering = severity. P164.1 → P164.9. Granular commits, CI green between each, single PR.

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
