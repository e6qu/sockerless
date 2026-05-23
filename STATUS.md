# Sockerless — Status

Roadmap [PLAN.md](PLAN.md) · resume [DO_NEXT.md](DO_NEXT.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · vibe catalogue [docs/VIBE_CODING.md](docs/VIBE_CODING.md).

## Snapshot

| | |
|---|---|
| Active branch | `phase-174-skill-sweep` — first audit pass using the 5 skills added in Phase 173.0. |
| In-flight | Phase 174 runs each new skill across the repo. The skills caught 4 quick-fix BUGs and 3 larger-scope follow-ups. Quick-fix BUGs already fixed in-branch: BUG-1105 (23 `_ = sim.ReadJSON(r, &req)` silent-decode sites across Phase 173 handlers — the very pattern silent-error-swallow-scan codifies, introduced after the skill was written; BUG-1104 meta-shape in action), BUG-1106 (silent decode in `handleKVCreateCertificate`), BUG-1107 (dead `var _ = aws.Config{}` import silencer in metadata_sdk_test.go), BUG-1108 (dead `singleSelectInputValueType` "reserved for future" silencer in bleephub/gh_projects_v2_graphql.go). Larger-scope follow-ups filed as Open: BUG-1109 (Azure File/Queue/Table data planes — same shape as BUG-1103 Blob, scope down to remaining 3), BUG-1110 (streaming-envelope sentinel-header inspection missing in GCS / Cloud Run invoke / ACR blob / Azure Blob PutBlob handlers — class of BUG-1099), BUG-1111 (Azure Functions URLs + AWS Amplify webhook URL + AWS ECR repositoryUri emitted but unrouted — `sim-emitted-url-roundtrip` shape). All regression tests green (AWS/GCP/Azure SDK suites + canonical-config). |
| Last merged | PR #179 — Phase 173 simulator wire-fidelity sweep (2026-05-24, squash `64a13a8`). |
| Standing merge auth | **None.** User merges every PR. |
| Cells | 8/8 runner-integration cells GREEN since 2026-05-07. |
| Bugs | 1111 filed · 1104 fixed · 5 open · 2 false positives. Open: BUG-1075 + BUG-1104 (pre-existing) + BUG-1109 + BUG-1110 + BUG-1111 (filed by Phase 174 sweep; tracked for follow-up phases). |
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
- **SDK/CLI/Terraform provider call sequences differ materially from each other** (BUG-1029/1030/1038-sub-fix/1095). Simulator endpoint-fidelity fixes need the real external clients, not internal shortcuts; one missing canonical field can surface only in gcloud or Terraform.
- **specs/CLOUD_RESOURCE_MAPPING.md is authoritative** for "how does sockerless model X on cloud Y."

### bleephub-specific
- **`gh` CLI is the reference adaptor.** If it works against `api.github.com`, it must work against bleephub. No URL hackery.
- **`gh` is HTTPS-only against non-`github.com` hosts.** Quick-start in `bleephub/README.md` covers the self-signed-cert + system-trust path.
- **GitHub Apps and OAuth Apps are separate concepts.** Distinct store entries, distinct token prefixes (`ghp_`/`gho_`/`ghu_`/`ghs_`/`ghr_`).
- **Installation tokens are immutable snapshots.** Re-mint to pick up perm changes.
- **Body coercion is per-GitHub-spec.** `flexBool` / `flexInt` accept both typed and string-coerced JSON (what `gh api -f` sends).
- **No `alg:none` JWTs in OAuth issuance** — BUG-1000.

## Phase 167 — Pod-model analysis + Phase 168 execution (merged)

User directive (2026-05-17): compare pod abstraction across 7 backends; trace runner ↔ backend call sequences; root-cause the "12-step CI job = 12+ min" symptom; design simplifications. Analysis only — no code edits.

Phase 167/168 deliverables:
- Cross-backend pod-model comparison: long-lived backends (docker/ecs/cloudrun/aca) hold one container/task/revision for the entire job; FaaS backends (lambda/gcf/azf) are invoke-on-demand. Per-backend exec dispatch differs in ways that the audit caught + codex review re-checked.
- Root cause of "12 steps = 12+ min": **Path B silent fallback in lambda + cloudrun + cloudrun-functions** dispatch. When the in-container reverse-agent doesn't dial back, every `docker exec` becomes a fresh function invocation cold-starting in 30-90s. 12 invocations × cold-start = the wall-clock symptom.
- Phase 168 implementation: unified exec on Model A (mandatory reverse-agent WebSocket; no Path B anywhere); default storage to in-memory tmpfs on cloudrun + cloudrun-functions + ACA (lambda + azf platforms reject `BackingMemory` so they keep current defaults); ripped all Path B code; ripped the parallel `core.CloudExecDriver` interface; cleanup failures propagate; FaaS pod lifetime is hard-capped at platform max.
- Driver model preserved: typed `core.ExecDriver` stays as the load-bearing abstraction. Each backend registers ONE driver matching its platform's primitive. Operator pluggability remains.

Codex review caught 3 corrections during Phase 167:
- AZF is Path A only (no Path B) — opposite of my initial claim.
- Tmpfs default scope must exclude lambda + azf (their volume translators reject `BackingMemory`).
- Tmpfs size clamping is itself a silent fallback (must fail-loud startup instead).

Self-caught during the "does the exec driver still make sense" check: **cloudrun ALSO has the Path A/B pattern**, missed in the initial Phase 167 analysis. Added to Phase 168 scope as BUG-1054.

User-confirmed for Phase 168: Model A; no fallbacks anywhere; FaaS max duration is hard limit (no extension hacks); `execStartViaInvoke` ripped entirely; cleanup failures propagate.

## Recently closed phases (last 6)

| PR | Phase | Headline |
|---|---|---|
| open #179 | 173 | Simulator wire-fidelity sweep — 20 commits (15 implementation + 5 CI-wrap: dep freshness, Makefile fanout, gofmt, 2 staticcheck/vet fixes), ~180 sim ops added across AWS/GCP/Azure, closes issues #173–#178 + BUG-1098..1104. **Draft; CI-green on `a448971` (all 11 jobs pass); awaiting user merge.** |
| #172 | pod-model follow-up | Simulator pod materialization fidelity: real multi-container execution + localhost sidecar SDK tests for ECS, Cloud Run Services/Jobs, ACA Jobs/Apps; AZF pod docs corrected to unsupported; real-runner simulator arithmetic targets added. Merged 2026-05-23 at `1c1fd92`. |
| #170 | 168 follow-up | FaaS runner smokes for Lambda/Cloud Run/GCF/ACA/AZF, Make/CI wiring, AZF bootstrap coverage, GCP Artifact Registry endpoint-fidelity fix covered by SDK/gcloud/Terraform/OCI, and live-validation runbook. Merged 2026-05-18 at `a5639811`. |
| #169 | 168 follow-up | Runner attach hardening and final CI stabilization. Merged 2026-05-18 at `0bd75902`. |
| #168 | 167–168 | FaaS exec unification, reverse-agent-only path, and AZF bootstrap hardening. Merged 2026-05-18 at `3565e413`. |
| #167 | 166 | Real fixes for Phase 165 follow-ups (4 BUGs: 1040 Azure azurerm + 1041 GCP IAM SA + 1042 AWS 5 sim handler gaps + 1045 codex state-persistence). Merged 2026-05-17 at `49050c2d`. |
| #165 | 165 | Third vibe-slop sweep + sim test-pyramid expansion + codex review + continuity-doc compression. 9 BUGs closed. Merged 2026-05-17 at `288b76d3`. |

Older phases (#112–#161): one-line headlines in [PLAN.md § Closed phases](PLAN.md); per-phase narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md).
