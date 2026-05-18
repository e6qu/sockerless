# Sockerless — Status

Roadmap [PLAN.md](PLAN.md) · resume [DO_NEXT.md](DO_NEXT.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · vibe catalogue [docs/VIBE_CODING.md](docs/VIBE_CODING.md).

## Snapshot

| | |
|---|---|
| Active branch | `phase-167-pod-model-analysis` — PR #168 open; keep the same branch and PR. |
| In-flight | **Phases 167 + 168 on the same branch.** P168.1–.8 landed; P168.9/CI hardening is in progress. Latest chunk: BUG-1069, BUG-1091, BUG-1092, and BUG-1093 are fixed locally. ACA Apps now wrap non-overlay images through ACR Tasks with `SOCKERLESS_ACA_BOOTSTRAP`, preserving user argv in runtime `SOCKERLESS_USER_*` env and using the reverse-agent bootstrap as ENTRYPOINT. CI run `26013037364` exposed a redundant GCF `alpine` registry pull; CI run `26013907371` showed the first fix still assumed BuildKit left the ECR Public base tag in the image store. GCF and Cloud Run now build the Docker Hub / AR alpine tags explicitly from the real ECR Public base. Full ACA validation exposed Azure simulator execution-container name collisions from 12-char execution-name truncation; the simulator now uses the full execution name. Remaining: commit/push, re-check CI, then continue with AZF bootstrap BUG-1067 and e2e follow-ups 1071–1076. External `codex review` remains blocked unless the user explicitly approves exporting repo diff/context. |
| Last merged | PR #167 — Phase 166 (2026-05-17, `49050c2d`). All Open BUGs closed at merge. |
| Standing merge auth | **None.** User merges every PR. |
| Cells | 8/8 runner-integration cells GREEN since 2026-05-07. |
| Bugs | 1093 filed · 1086 fixed · 7 open · 2 false positives. Open Phase 168 blockers are BUG-1067 / 1071 plus runner/live/test-pyramid follow-ups 1072–1076. |
| Live infra | None up. |

## Invariants (carry across compactions / fresh sessions)

### Process
- **Never auto-merge PRs.** Push, wait for `gh pr checks` green, ping user. One-time exceptions don't carry forward.
- **Single-branch rule.** All in-flight work for one phase lands on one branch; many granular commits, one PR.
- **File BUGs *before* fixing.** Survey first, write `BUGS.md § Open` entries, only then start fix commits.
- **State save every task.** STATUS.md + DO_NEXT.md + WHAT_WE_DID.md + MEMORY.md + `_tasks/done/`.
- **Test all the time.** `go test ./...` in every touched module; harness-touch re-runs the harness; terraform-touch runs `terragrunt validate`.
- **Verify each significant chunk.** Don't batch fixes; commit + run tests + push between sub-tasks so CI catches regressions early.
- **Branch hygiene.** Rebase phase branch on `origin/main` before pushing; sync local `main` after merge.
- **Pre-push hooks own the truth.** If `check-latest-deps` flags dep drift, bump deps in the same branch — never skip the hook.
- **Read `.claude/skills/avoid-vibe-slop/SKILL.md` before every non-trivial change** — the catalogue exists to apply at write-time.

### Architecture
- **Components stay decoupled from admin / UI.** Sims, backends, bleephub run independently via env vars; admin reads only `/v1/health`, `/v1/info`, env.
- **Backend ↔ host primitive must match.** ECS in ECS, Lambda in Lambda, Cloud Run in Cloud Run, GCF in CRF, ACA in ACA, AZF in AZF.
- **No fakes / no fallbacks.** Unknown values fail loud. Operator-requested persistence + auth never silently degrade.
- **Persistence is opt-in + fail-loud.** `BLEEPHUB_PERSIST=true` / `SIM_PERSIST=true` → SQLite. Open-failure *and* write-failure must surface (BUG-985/986 + BUG-997); never silent in-memory fallback.
- **HTTP handlers in `backends/core/handle_*.go` must dispatch through `s.self.<Method>`** — never read `s.Store.*` directly (BUG-991/992/995).
- **Test target gating.** Backend integration tests require `SOCKERLESS_TEST_TARGET=sim|cloud`; never implicit skip.
- **No phase or bug IDs in code comments or test docstrings** (BUG-994/1014/1026/1036). Metadata lives in commits / PRs / BUGS.md; comments document the *why*.
- **Terraform provider call sequences differ materially from raw SDK** (BUG-1029/1030/1038-sub-fix). Both test layers required; one missing canonical field surfaces only live.
- **specs/CLOUD_RESOURCE_MAPPING.md is authoritative** for "how does sockerless model X on cloud Y."

### bleephub-specific
- **`gh` CLI is the reference adaptor.** If it works against `api.github.com`, it must work against bleephub. No URL hackery.
- **`gh` is HTTPS-only against non-`github.com` hosts.** Quick-start in `bleephub/README.md` covers the self-signed-cert + system-trust path.
- **GitHub Apps and OAuth Apps are separate concepts.** Distinct store entries, distinct token prefixes (`ghp_`/`gho_`/`ghu_`/`ghs_`/`ghr_`).
- **Installation tokens are immutable snapshots.** Re-mint to pick up perm changes.
- **Body coercion is per-GitHub-spec.** `flexBool` / `flexInt` accept both typed and string-coerced JSON (what `gh api -f` sends).
- **No `alg:none` JWTs in OAuth issuance** — BUG-1000.

## Phase 167 — Pod-model analysis + Phase 168 plan (doc-only; in flight)

User directive (2026-05-17): compare pod abstraction across 7 backends; trace runner ↔ backend call sequences; root-cause the "12-step CI job = 12+ min" symptom; design simplifications. Analysis only — no code edits.

Phase 167 deliverables (this branch):
- Cross-backend pod-model comparison: long-lived backends (docker/ecs/cloudrun/aca) hold one container/task/revision for the entire job; FaaS backends (lambda/gcf/azf) are invoke-on-demand. Per-backend exec dispatch differs in ways that the audit caught + codex review re-checked.
- Root cause of "12 steps = 12+ min": **Path B silent fallback in lambda + cloudrun + cloudrun-functions** dispatch. When the in-container reverse-agent doesn't dial back, every `docker exec` becomes a fresh function invocation cold-starting in 30-90s. 12 invocations × cold-start = the wall-clock symptom.
- Phase 168 plan (in this file's Active phase section): unify exec on Model A (mandatory reverse-agent WebSocket; no Path B anywhere); default storage to in-memory tmpfs on cloudrun + cloudrun-functions + ACA (lambda + azf platforms reject `BackingMemory` so they keep current defaults); rip all Path B code; rip the parallel `core.CloudExecDriver` interface; cleanup failures propagate; FaaS pod lifetime hard-capped at platform max.
- Driver model preserved: typed `core.ExecDriver` stays as the load-bearing abstraction. Each backend registers ONE driver matching its platform's primitive. Operator pluggability remains.

Codex review caught 3 corrections during Phase 167:
- AZF is Path A only (no Path B) — opposite of my initial claim.
- Tmpfs default scope must exclude lambda + azf (their volume translators reject `BackingMemory`).
- Tmpfs size clamping is itself a silent fallback (must fail-loud startup instead).

Self-caught during the "does the exec driver still make sense" check: **cloudrun ALSO has the Path A/B pattern**, missed in the initial Phase 167 analysis. Added to Phase 168 scope as BUG-1054.

User-confirmed for Phase 168: Model A; no fallbacks anywhere; FaaS max duration is hard limit (no extension hacks); `execStartViaInvoke` ripped entirely; cleanup failures propagate.

User-pending for Phase 168: 6 sizing / disposition questions in DO_NEXT.md.

## Recently closed phases (last 5)

| PR | Phase | Headline |
|---|---|---|
| #167 | 166 | Real fixes for Phase 165 follow-ups (4 BUGs: 1040 Azure azurerm + 1041 GCP IAM SA + 1042 AWS 5 sim handler gaps + 1045 codex state-persistence). Merged 2026-05-17 at `49050c2d`. |
| #165 | 165 | Third vibe-slop sweep + sim test-pyramid expansion + codex review + continuity-doc compression. 9 BUGs closed. Merged 2026-05-17 at `288b76d3`. |
| #164 | 164 | Second vibe-slop sweep + terraform-provider test expansion (19 BUGs). Merged 2026-05-17 at `616dcd98`. |
| #163 | 163 | Makefile legacy alias rip-out + docs sweep. Merged 2026-05-16 at `d5b9d22a`. |
| #162 | 162 | Vibe-coding catalogue refresh (12 new patterns 24–35). Doc-only. Merged 2026-05-16 at `4f602988`. |

Older phases (#112–#161): one-line headlines in [PLAN.md § Closed phases](PLAN.md); per-phase narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md).
