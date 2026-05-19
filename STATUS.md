# Sockerless — Status

Roadmap [PLAN.md](PLAN.md) · resume [DO_NEXT.md](DO_NEXT.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · vibe catalogue [docs/VIBE_CODING.md](docs/VIBE_CODING.md).

## Snapshot

| | |
|---|---|
| Active branch | `pod-model-simulator-fidelity` — pod materialization + simulator fidelity follow-up. |
| In-flight | BUG-1096 fixed in-tree: AWS ECS, GCP Cloud Run Services/Jobs, and Azure ACA Jobs/Apps simulators now start every declared pod/task container as a real local workload and use shared network namespaces for localhost sidecar semantics. Added SDK regression coverage across AWS/GCP/Azure, corrected AZF pod docs to match the actual single-container Function App implementation, and wired real GitHub/GitLab runner arithmetic harness targets against a caller-started simulator-backed sockerless daemon. PR #172 CI run `26063005479` surfaced BUG-1097 in AWS ECS task-container naming / sidecar host config; fixed and locally verified with the focused failing tests plus the full AWS SDK target. Push hook also required dependency freshness cleanup: `bleephub` now uses `github.com/go-git/go-git/v5 v5.19.1` and README line-count badges were refreshed. Remaining tracked implementation follow-up: BUG-1075 live-cloud validation, which requires real credentials/setup and must not be marked complete without a real run. |
| Last merged | PR #170 — Phase 168 follow-up runner smokes and simulator fidelity (2026-05-18, `a5639811`). |
| Standing merge auth | **None.** User merges every PR. |
| Cells | 8/8 runner-integration cells GREEN since 2026-05-07. |
| Bugs | 1097 filed · 1095 fixed · 1 open · 2 false positives. Only BUG-1075 remains open from the Phase 168 follow-up list. |
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

## Recently closed phases (last 5)

| PR | Phase | Headline |
|---|---|---|
| open | pod-model follow-up | Simulator pod materialization fidelity: real multi-container execution + localhost sidecar SDK tests for ECS, Cloud Run Services/Jobs, ACA Jobs/Apps; AZF pod docs corrected to unsupported; real-runner simulator arithmetic targets added. |
| #170 | 168 follow-up | FaaS runner smokes for Lambda/Cloud Run/GCF/ACA/AZF, Make/CI wiring, AZF bootstrap coverage, GCP Artifact Registry endpoint-fidelity fix covered by SDK/gcloud/Terraform/OCI, and live-validation runbook. Merged 2026-05-18 at `a5639811`. |
| #169 | 168 follow-up | Runner attach hardening and final CI stabilization. Merged 2026-05-18 at `0bd75902`. |
| #168 | 167–168 | FaaS exec unification, reverse-agent-only path, and AZF bootstrap hardening. Merged 2026-05-18 at `3565e413`. |
| #167 | 166 | Real fixes for Phase 165 follow-ups (4 BUGs: 1040 Azure azurerm + 1041 GCP IAM SA + 1042 AWS 5 sim handler gaps + 1045 codex state-persistence). Merged 2026-05-17 at `49050c2d`. |
| #165 | 165 | Third vibe-slop sweep + sim test-pyramid expansion + codex review + continuity-doc compression. 9 BUGs closed. Merged 2026-05-17 at `288b76d3`. |

Older phases (#112–#161): one-line headlines in [PLAN.md § Closed phases](PLAN.md); per-phase narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md).
