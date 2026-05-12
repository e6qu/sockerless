# Do Next

Status [STATUS.md](STATUS.md) · roadmap [PLAN.md](PLAN.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Where we are

Phase 157 (component ⇄ reference-adaptor docs sweep) starting on `phase-157-component-adaptor-docs`, off `origin/main` post-#156 merge.

PRs #153 → #154 → #155 → #156 all merged 2026-05-12 → 2026-05-13. Standing merge authorization is **spent** — every Phase 157 commit goes through the normal "push, wait for user merge" loop.

## Phase 157 — Component ⇄ reference-adaptor docs sweep

### Frame

Every component in this repo is paired with an external **reference adaptor**. The adaptor is simultaneously:

- **Validation** — test harnesses drive the real adaptor against the component.
- **Utility** — adaptor is how end-users actually invoke the component.
- **Reference** — adaptor's docs/behaviour define what "correct" means for the component.

Doc structure per component (lead with the adaptor, not the binary):

```
# <component>

## Reference adaptor
This component is measured against <upstream tool>, version <X+>.
The contract: anything <upstream tool> does against <upstream service>,
it must do against this component.

## Validation
Test harness: <path>. Last green: <N/N PASS>.

## Wiring the adaptor
<2–5 lines of env / config>

## Sample
$ <command>
<real captured output>

## What's out of scope
<deferred items>
```

### Adaptor matrix

| Component | Reference adaptor | Validation entry point |
|---|---|---|
| `backends/docker` | docker CLI / podman CLI / Docker Go SDK | `tests/` (Docker SDK e2e, 59 tests) |
| `backends/ecs` | aws CLI/SDK + Terraform aws (cloud); docker CLI (frontend) | `simulators/aws/sdk-tests` + Docker SDK e2e |
| `backends/lambda` | aws CLI/SDK + Terraform aws; docker CLI | same |
| `backends/cloudrun` | gcloud + Go SDK + Terraform google; docker CLI | `simulators/gcp/sdk-tests` |
| `backends/cloudrun-functions` | gcloud + Go SDK + Terraform google; docker CLI | same |
| `backends/aca` | az + Go SDK + Terraform azurerm; docker CLI | `simulators/azure/sdk-tests` |
| `backends/azure-functions` | az + Go SDK + Terraform azurerm; docker CLI | same |
| `simulators/aws` | aws CLI + AWS Go SDK + Terraform aws | `simulators/aws/{sdk-tests,terraform-tests}` |
| `simulators/gcp` | gcloud + Go SDK + Terraform google | `simulators/gcp/{sdk-tests,terraform-tests}` |
| `simulators/azure` | az + Go SDK + Terraform azurerm | `simulators/azure/{sdk-tests,terraform-tests}` |
| `bleephub` | gh CLI | `bleephub/test/run-gh-test.sh` (✅ already documented in #155 + #156) |
| `cmd/sockerless` (CLI) | itself — CLI is adaptor for backends | `cmd/sockerless/*_test.go` |
| `cmd/sockerless-admin` | browser / REST clients against `/v1/*` | `cmd/sockerless-admin/*_test.go` |

### Commit layout

Single branch `phase-157-component-adaptor-docs`. Granular commits so CI runs per increment, reverts are surgical:

1. **State save** (this commit set: STATUS / DO_NEXT / PLAN / BUGS / WHAT_WE_DID).
2. `backends/docker` — simplest case, no cloud connector.
3. `backends/{ecs,lambda}` — AWS cluster.
4. `backends/{cloudrun,cloudrun-functions}` — GCP cluster.
5. `backends/{aca,azure-functions}` — Azure cluster.
6. `simulators/{aws,gcp,azure}` — per-simulator READMEs.
7. `simulators/README.md` — the **end-to-end showcase** (3 loop variants).
8. `cmd/sockerless/README.md` + `cmd/sockerless-admin/README.md` — CLI + admin updates.

### Defaults (assumed unless user redirects)

- **Podman**: documented as "drop-in via `DOCKER_HOST`," not a separate adaptor track.
- **One doc per component**, leading with the adaptor section (not the binary).
- **Real captured output** in every sample block — run each command, paste actual output. Truncate `terraform apply` to headline + diff stats + final `Apply complete!`.

### Acceptance bar

- Every connector example is **copy-pasteable** against current `main`.
- Every "expected output" block reflects what the binary actually prints (run it, paste it, don't guess).
- Every prereq links to the upstream install doc.
- End-to-end showcase: 3 working variants, each ≤15 lines of bash from zero to a `docker run` round-trip through a simulator.
- No "Phase 8X TODO" dangling placeholders in any touched doc.

### Out of scope

- Live-cloud cells — covered by `docs/ECS_LIVE_SETUP.md` and the live tracks.
- Any code changes — docs only (state save is "docs" in this context).
- bleephub — just got two full passes in #155 + #156.

## Resumable tracks after Phase 157 merges

### Track A — Live-cloud validation

Lambda live · Cloud Run Services + ACA Apps live · AZF cloud-dns live · Lambda service-mesh live · ACA/AZF Azure AD live. One branch per cell. Teardown self-sufficient per `feedback_teardown_aggressive.md`.

### Track B — Phase 91d (bookmarked indefinitely)

Real `pd-ephemeral` lifecycle on cloudrun + gcf. Cloud Run lacks the protobuf field. Don't reopen until cloud capability changes.

### Track C — bleephub persistence expansion

Phases 153–156 ship SQLite for users / tokens / apps / oauth_apps / installations / installation_tokens / user_to_server_tokens / refresh_tokens / repos. Extending to issues / PRs / hooks / hook deliveries / check_runs / check_suites / labels / milestones / comments / secrets / orgs / teams / memberships is a separate phase once a real use case surfaces. Git storage (go-git) → `filesystem.Storage` is its own phase.

## Session-resume checklist

1. `git fetch origin && git checkout phase-157-component-adaptor-docs && git pull` (or `git checkout main && git pull --ff-only` if 157 has merged).
2. `git log --oneline -10` to see what's already on the branch.
3. Read STATUS.md (snapshot) + this file (concrete next actions) + the **adaptor matrix** above.
4. `go test ./...` in any touched module to confirm green baseline.
5. Pick the next un-committed row from the commit layout; write that doc; commit; push; let CI run.
6. File BUGS.md entries for anything that surfaces; fix in the same session.
7. State-save before pushing: STATUS.md + this file + WHAT_WE_DID.md + MEMORY.md.

## Invariants snapshot (full list in STATUS.md)

- Never auto-merge; user merges every PR.
- Components decoupled from admin / UI.
- No fakes / no fallbacks / no silent shims.
- Backend ↔ host primitive must match.
- `gh` CLI is the reference adaptor for bleephub; HTTPS-only, `--hostname` is the wiring flag.
- `specs/CLOUD_RESOURCE_MAPPING.md` is authoritative for the cloud-mapping table.
