# Sockerless — Status

Roadmap [PLAN.md](PLAN.md) · resume [DO_NEXT.md](DO_NEXT.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · vibe catalogue [docs/VIBE_CODING.md](docs/VIBE_CODING.md).

## Snapshot

| | |
|---|---|
| Active branch | `phase-165-vibe-slop-sweep-3-test-pyramid` — PR TBD. |
| In-flight | **Phase 165 — third vibe-slop sweep + sim test-pyramid expansion + continuity-doc compression.** 9 BUGs closed in this PR (1033–1036 vibe-slop + 1038/1039 GCP + Azure terraform expansions + 1043/1044 codex review findings) — also surfaced + closed a GCS-object selfLink sub-defect during the BUG-1038 expansion. 3 BUGs filed as Open for Phase 166 follow-up: 1040 (azurerm research for ACA/AZF/ACR), 1041 (GCP IAM SA + Cloud Functions Gen2), 1042 (AWS S3/DynamoDB/KMS/SM/SSM — 5 sim handler gaps surfaced). |
| Last merged | PR #164 — Phase 164 second vibe-slop sweep + terraform-provider test expansion (2026-05-17, `616dcd98`). |
| Standing merge auth | **None.** User merges every PR. |
| Cells | 8/8 runner-integration cells GREEN since 2026-05-07. |
| Bugs | 1041 fixed · 3 open (1040/1041/1042) · 2 false positives. |
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

## Phase 165 — Third vibe-slop sweep + test-pyramid expansion + continuity-doc compression (in flight)

User directive (2026-05-17): re-run vibe-slop on a fresh main; plan test-pyramid expansion against real adaptors (SDK + terraform-provider + CLI); single PR with sub-phases; verify after every significant chunk; prune obsolete continuity-doc info for cross-compaction durability; codex CLI review at end.

Closed in this PR (9 BUGs):
- **Vibe-slop sweep #3** — silent `io.Copy` swallows in image-stream + build response paths (1033); dead `fmt.Sprintf` silencer + lying comment in lambda e2e test (1034); `w.Write` style inconsistency at 3 outlier sites (1035); ~50 test-file docstrings carrying Phase / sub-phase metadata (1036).
- **Test-pyramid expansion** — Azure terraform-tests + storage_account + key_vault (1039); GCP terraform-tests + subnet + firewall + GCS object + a surfaced sim-defect-fix on GCS object selfLink/id/mediaLink (1038).
- **Codex CLI review findings** — `account_kind="StorageV2"` rejected by azurestack provider at plan time (1043); GCS object URL not percent-encoded for names containing `/`/space/`?` (1044).

Staged forward as Open BUGs for Phase 166:
- **1040** — Azure terraform-tests azurerm-research for ACA + AZF + ACR + Application Insights + identity + private DNS + Key Vault data-plane.
- **1041** — GCP terraform-tests follow-up: IAM SA, Cloud Functions Gen2, Compute instance/instance_template, Cloud Build trigger, Logging sink/metric, Pub/Sub.
- **1042** — AWS terraform-tests: first attempt surfaced 5 distinct sim handler gaps (S3 path-style mismatch, DynamoDB DescribeTable shape, KMS GetKeyPolicy, SecretsManager GetResourcePolicy, SSM ListTagsForResource); each needs its own commit with sim handler additions.

## Recently closed phases (last 5)

| PR | Phase | Headline |
|---|---|---|
| #164 | 164 | Second vibe-slop sweep + terraform-provider test expansion (19 BUGs: 1014–1032). GCP 4 → 11 resources; Azure 1 → 5. Merged 2026-05-17 at `616dcd98`. |
| #163 | 163 | Makefile legacy alias rip-out + docs sweep. Merged 2026-05-16 at `d5b9d22a`. |
| #162 | 162 | Vibe-coding catalogue refresh — `docs/VIBE_CODING.md` 23 → 35 patterns; `avoid-vibe-slop` skill 17 → 26 items. Doc-only. Merged 2026-05-16 at `4f602988`. |
| #161 | 161 | First comprehensive vibe-slop sweep — 18 BUGs closed + bleephub GraphQL completion + ProjectManager instance-based lifecycle rewrite. Merged 2026-05-16 at `841f2456`. |
| #160 | 160 | Project-local Claude skills (`sim-handler-checklist`, `cross-resource-stack-test`) + component-README adaptor-led sweep. Merged 2026-05-16 at `aeb0ac6e`. |

Older phases (#112–#159): one-line headlines in [PLAN.md § Closed phases](PLAN.md); per-phase narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md).
