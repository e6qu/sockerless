# Do Next

Status [STATUS.md](STATUS.md) · roadmap [PLAN.md](PLAN.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md) · vibe-coding catalogue [docs/VIBE_CODING.md](docs/VIBE_CODING.md).

## Where we are

Phase 158 (BUG-991 fix + VIBE_CODING.md sourced catalogue + 3 project-local Claude skills) shipping on `phase-158-bug991-vibecoding-skills`. PR #157 merged 2026-05-13.

## Phase 158 status

| Sub-task | Status | Notes |
|---|---|---|
| P158.1 — BUG-991 fix (delegate ContainerWait to s.self) | ✅ | `handle_containers.go` + `BaseServer.ContainerWait`; verified manually; `go test ./...` green in `backends/core/`. |
| P158.2 — Audit other fallback-hiding-bugs | ✅ | BUG-992 (list endpoints) filed and staged as Phase 159. |
| P158.3 — `docs/VIBE_CODING.md` | ✅ | 23-pattern sourced catalogue. |
| P158.4 — Claude skills | ✅ | `avoid-vibe-slop`, `adaptor-fidelity-check`, `manual-test` under `.claude/skills/`. |
| P158.5 — State save | in-progress | This commit. |

Remaining: commit + push + open PR + wait for user merge.

## Resumable tracks after Phase 158 merges

### Track A — Phase 159 — Passthrough-list-endpoints sweep

BUG-992 fix. Audit + fix every list endpoint that reads `s.Store.X.List()` directly without consulting `s.self.X`. Known affected: `handle_images.go handleImageList`, `handle_volumes.go` list, `handle_networks.go` list. Likely more. Cross-cloud sweep on every find.

Acceptance: `DOCKER_HOST=tcp://localhost:3375 docker {images,volume ls,network ls}` returns the upstream daemon's actual resources against `backends/docker`.

### Track B — Resume Phase 157 component-adaptor sweep (deferred during 158)

Phase 157 PR #157 only covered `backends/docker`. The other components (backends/{ecs,lambda,cloudrun,cloudrun-functions,aca,azure-functions}, simulators/{aws,gcp,azure}, simulators/README.md end-to-end showcase, cmd/sockerless, cmd/sockerless-admin) need the same adaptor-led rewrite. Component matrix below.

Per-component plan from PR #157:

| Component | Reference adaptor | Validation entry point |
|---|---|---|
| `backends/ecs` | aws CLI/SDK + Terraform aws; docker CLI | `simulators/aws/sdk-tests` + Docker SDK e2e |
| `backends/lambda` | aws CLI/SDK + Terraform aws; docker CLI | same |
| `backends/cloudrun` | gcloud + Go SDK + Terraform google; docker CLI | `simulators/gcp/sdk-tests` |
| `backends/cloudrun-functions` | gcloud + Go SDK + Terraform google; docker CLI | same |
| `backends/aca` | az + Go SDK + Terraform azurerm; docker CLI | `simulators/azure/sdk-tests` |
| `backends/azure-functions` | az + Go SDK + Terraform azurerm; docker CLI | same |
| `simulators/aws` | aws CLI + AWS Go SDK + Terraform aws | `simulators/aws/{sdk-tests,terraform-tests}` |
| `simulators/gcp` | gcloud + Go SDK + Terraform google | `simulators/gcp/{sdk-tests,terraform-tests}` |
| `simulators/azure` | az + Go SDK + Terraform azurerm | `simulators/azure/{sdk-tests,terraform-tests}` |
| `cmd/sockerless` (CLI) | itself — CLI is the adaptor for backends | `cmd/sockerless/*_test.go` |
| `cmd/sockerless-admin` | browser / REST clients against `/v1/*` | `cmd/sockerless-admin/*_test.go` |

Doc shape (locked-in from #157): lead with adaptor, then validation, wiring, sample (real captured output), out-of-scope.

Headline pending: `simulators/README.md` end-to-end showcase — 3 loop variants (AWS sim ↔ ECS backend, GCP sim ↔ Cloud Run backend, Azure sim ↔ ACA backend), each ≤15 lines of bash to `docker run alpine echo hi` round-tripping through a real simulator.

### Track C — Skill maturation (post-Phase 158)

The three skills (`avoid-vibe-slop`, `adaptor-fidelity-check`, `manual-test`) are v1. As we surface more patterns, append to `docs/VIBE_CODING.md` and reference them from the skill checklists. Candidate additional skills (future phases):

- `state-save` — codify the STATUS/PLAN/DO_NEXT/BUGS/WHAT_WE_DID refresh rhythm.
- `spec-first-implementation` — verify spec exists in `specs/` before coding.
- `cross-cloud-sweep` — formal procedure for the "if found in one backend, check the other 5" rule.

### Track D — Live-cloud validation

Lambda live · Cloud Run Services + ACA Apps live · AZF cloud-dns live · Lambda service-mesh live · ACA/AZF Azure AD live. One branch per cell.

### Track E — Phase 91d (bookmarked indefinitely)

Real `pd-ephemeral` on cloudrun + gcf. Don't reopen until cloud capability changes.

## Invariants snapshot (full list in STATUS.md + VIBE_CODING.md)

- Never auto-merge; user merges every PR.
- Components decoupled from admin / UI.
- No fakes / no fallbacks / no silent shims.
- Backend ↔ host primitive must match.
- `gh` CLI is the reference adaptor for bleephub; HTTPS-only, `--hostname` is the wiring flag.
- `specs/CLOUD_RESOURCE_MAPPING.md` is authoritative for cloud-mapping.
- Read `docs/VIBE_CODING.md` before any non-trivial code change.
- Read `.claude/skills/avoid-vibe-slop/SKILL.md` checklist before writing handlers / tests / "fixes".

## Session-resume checklist

1. `git fetch origin && git checkout phase-158-bug991-vibecoding-skills && git pull` (or `git checkout main && git pull --ff-only` if 158 merged).
2. `git log --oneline -10` to see what's on the branch.
3. Read STATUS.md + this file + the recent commits.
4. If editing code: read `.claude/skills/avoid-vibe-slop/SKILL.md` and run its checklist.
5. If editing a wire-facing handler: also read `.claude/skills/adaptor-fidelity-check/SKILL.md`.
6. Manual test before claiming done (per `.claude/skills/manual-test/SKILL.md`).
7. File BUGS.md entries for anything that surfaces; fix in the same session.
8. State-save before pushing.
