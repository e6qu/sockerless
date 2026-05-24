# Sockerless — Status

Roadmap [PLAN.md](PLAN.md) · resume [DO_NEXT.md](DO_NEXT.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · vibe catalogue [docs/VIBE_CODING.md](docs/VIBE_CODING.md).

## Snapshot

| | |
|---|---|
| Active branch | `phase-178-community-issues` — PR #211 open + awaiting CI + user merge. 16 commits closed BUG-1148..1160 (1 reopen + 8 community-filed + 4 class-of-bug remediations). |
| In-flight | PR #211 — CI running on 22 jobs. |
| Last merged | PR #202 — Phase 177 community-filed issues + 4 meta-skill improvements (2026-05-24, squash `aa847b1`). |
| Standing merge auth | **None.** User merges every PR. |
| Cells | 8/8 runner-integration cells GREEN since 2026-05-07. |
| Bugs | 1160 filed · 1158 fixed · 2 open · 2 false positives. Open: BUG-1075 (live-cloud) + BUG-1104 (audit-cadence meta). |
| Live infra | None up. |

## Invariants (carry across compactions / fresh sessions)

### Process
- **Never auto-merge PRs.** Push, wait for `gh pr checks` green, ping user.
- **Single-branch rule.** All in-flight work for one phase lands on one branch; many granular commits, one PR.
- **File BUGs *before* fixing.** Survey first, write `BUGS.md § Open` entries, only then start fix commits.
- **State save every task.** STATUS.md + DO_NEXT.md + WHAT_WE_DID.md + MEMORY.md.
- **Test all the time.** `go test ./...` in every touched module; harness-touch re-runs the harness; terraform-touch runs `terragrunt validate`.
- **Branch hygiene.** Rebase phase branch on `origin/main` before pushing; sync local `main` after merge.
- **Pre-push hooks own the truth.** If `check-latest-deps` flags dep drift, bump deps in the same branch — never skip.
- **Read `.claude/skills/avoid-vibe-slop/SKILL.md` before every non-trivial change.**

### Architecture
- **Components stay decoupled from admin / UI.** Sims, backends, bleephub run independently via env vars; admin reads only `/v1/health`, `/v1/info`, env.
- **Backend ↔ host primitive must match.** ECS in ECS, Lambda in Lambda, Cloud Run in Cloud Run, GCF in CRF, ACA in ACA, AZF in AZF.
- **No fakes / no fallbacks.** Unknown values fail loud. Operator-requested persistence + auth never silently degrade.
- **Persistence is opt-in + fail-loud.** `BLEEPHUB_PERSIST=true` / `SIM_PERSIST=true` → SQLite. Open-failure *and* write-failure must surface.
- **HTTP handlers in `backends/core/handle_*.go` must dispatch through `s.self.<Method>`** — never read `s.Store.*` directly.
- **Test target gating.** Backend integration tests require `SOCKERLESS_TEST_TARGET=sim|cloud`; never implicit skip.
- **No phase / bug IDs in code comments or test docstrings.** Metadata lives in commits / PRs / BUGS.md; comments document the *why*.
- **SDK / CLI / Terraform-provider call sequences differ materially.** Simulator endpoint-fidelity fixes need the real external clients, not internal shortcuts.
- **`specs/CLOUD_RESOURCE_MAPPING.md` is authoritative** for "how does sockerless model X on cloud Y."
- **Closed enumeration → full-table audit before "fixed".** Surface tables live in `specs/SIM_SURFACE_TABLES/` (46+ tables seeded in Phase 178). No silent ✗ rows.
- **Every reopen carries a postmortem trail** (what test passed but should have failed; what SDK code path was missed; what new canonical-client test catches the regression). P0 by default.
- **Mux pattern overlap is a real-bug class.** Collapsed-port sim has no DNS-level service isolation; `scripts/scan-mux-overlap.sh` runs in pre-commit (warn mode).
- **List* ops use paged-iterator tests.** Single-record envelopes silently pass `.Value[0]`-style tests.
- **Stateful resources model their state machine.** `sim-state-machine-completeness` skill audits every row of every surface table whose handler is stateful.

### bleephub-specific
- **`gh` CLI is the reference adaptor.** No URL hackery.
- **`gh` is HTTPS-only against non-`github.com` hosts.** Quick-start covers self-signed-cert + system-trust path.
- **GitHub Apps and OAuth Apps are separate concepts** with distinct token prefixes (`ghp_` / `gho_` / `ghu_` / `ghs_` / `ghr_`).
- **No `alg:none` JWTs in OAuth issuance.**

## Recently closed phases

| PR | Phase | Headline |
|---|---|---|
| #211 | 178 (in flight) | 9 community-filed issues (#196 reopen + #203..#210) + 4 class-of-bug remediations: proactive surface-table seed (46 tables) + mux-overlap scanner + paged-iterator rule + state-machine skill. 13 BUGs closed (1148-1160). |
| #202 | 177 | KV WWW-Authenticate URL reopen (#193) + S3 bucket-subresources (#201) + 4 meta-skill improvements (sim-canonical-config-test extension, surface-table-completeness, reopen-postmortem, tf-tests parity). 6 BUGs closed (1142-1147). Merged at `aa847b1`. |
| #200 | 176 | 8 community-filed issues (#190 reopened + #193..#199) + 12 in-PR audit findings + path-style storage dispatcher contamination + `make hooks`. 8 BUGs closed (1134-1141). Merged at `2d8e604`. |
| #192 | 175 | Second skill-sweep audit + 3 community-filed issues (#189/#190/#191) + signal-driven FaaS smoke tests + CI test-job split + new `timeless-comments` skill. 14 BUGs closed (1120-1133). Merged at `ca11405`. |
| #180 | 174 | Skill-sweep audit + community-issue triage — 3 rounds; 15 BUGs closed (1105–1119). Merged at `7a5d588`. |
| #179 | 173 | Simulator wire-fidelity sweep — 20 commits, ~180 sim ops added; closes #173–#178 + BUG-1098..1104. Merged at `64a13a8`. |

Older phases (#112–#172): one-line headlines in [PLAN.md § Closed phases](PLAN.md); per-phase narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md).
