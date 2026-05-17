# Do Next

Status [STATUS.md](STATUS.md) · roadmap [PLAN.md](PLAN.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · vibe catalogue [docs/VIBE_CODING.md](docs/VIBE_CODING.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Where we are

Phase 164 merged 2026-05-17 (PR #164, `616dcd98` on `origin/main`) — second vibe-slop sweep + terraform-provider test expansion (19 BUGs).

**Phase 165 in flight on `phase-165-vibe-slop-sweep-3-test-pyramid`** — third vibe-slop sweep + sim test-pyramid expansion + continuity-doc compression. 7 BUGs filed up front (1033–1039); sub-task table below. Single PR. Verify after every significant chunk.

Default "never auto-merge / user merges every PR" remains in force.

## Phase 165 sub-task table (severity-ordered)

| Sub | Status | BUG(s) | What |
|---|---|---|---|
| **P165.0** | ✅ | — | Branch from `origin/main@616dcd98` + survey + 7 BUGs (1033–1039) filed in `BUGS.md` + continuity-doc opening. |
| **P165.1** | ✅ | 1033 | `backends/core/{build.go:515,handle_images.go:80/115/472/493}` — 5 silent `io.Copy(w, rc)` swallows on image-stream + build response paths. Now `s.Logger.Debug().Err(err).Msg("<op> stream copy failed — client likely disconnected")`. |
| **P165.2** | ✅ | 1034 | Dropped `var _ = fmt.Sprintf` silencer + `fmt` import in `backends/lambda/agent_e2e_integration_test.go`; the claimed-consumer demuxer never called fmt. |
| **P165.3** | ✅ | 1035 | Standardised on `_, _ = w.Write(...)` at `bleephub/gh_oauth.go:171`, `bleephub/artifacts.go:321`, `simulators/azure/functions.go:290`. |
| **P165.4** | ✅ | 1036 | Rewrote ~50 test-file docstrings to describe the contract under test, not the phase. Preserved the runner-lifecycle phase labels in `tests/gitlab_runner_e2e_test.go` (docker-compose lifecycle phases, not project phases). |
| **P165.5** | ✅ | 1039 | Azure terraform-tests expanded with `azurestack_storage_account` + `azurestack_key_vault` (7 resources total). The wider ACA/AZF/ACR/AppInsights surface needs `azurerm` provider research → filed as BUG-1040 for Phase 166. |
| **P165.6** | ✅ | 1038 | GCP terraform-tests expanded with `google_compute_subnetwork` + `google_compute_firewall` + `google_storage_bucket_object` (15 resources total). Surfaced + fixed a sim defect (GCS object selfLink/id/mediaLink missing). IAM SA + Cloud Functions Gen2 follow-up → BUG-1041. |
| **P165.7** | ✅ | 1037 → 1042 | AWS terraform-test expansion attempt surfaced 5 distinct sim handler gaps (S3 path-style, DynamoDB DescribeTable shape, KMS GetKeyPolicy, SecretsManager GetResourcePolicy, SSM ListTagsForResource). Reverted the main.tf additions; filed BUG-1042 for Phase 166. |
| **P165.6.5** | ✅ | 1043 + 1044 | Ran `codex review --base main`. Two validated findings: azurestack provider rejects `account_kind="StorageV2"` at plan time (fixed → `"Storage"`); GCS sim selfLink/mediaLink missing `url.PathEscape` on object name (fixed). |
| **P165.8** | ✅ | — | Continuity-doc compression: STATUS / DO_NEXT / PLAN / WHAT_WE_DID compressed from ~1700 → ~870 lines (46% reduction); closed-phase per-sub-task tables + duplicate narrative pruned; invariants + active-phase scope + last-3-phase headlines + forward tracks kept. |
| **P165.9** | ◻ | — | Final state save: STATUS / DO_NEXT / WHAT_WE_DID / PLAN / MEMORY updated for any post-compression edits. |
| **P165.10** | ◻ | — | Push branch, open PR #165, wait for 11 standard CI checks green per push, ping user for merge. |

## Verification discipline

- `go test ./...` in every touched Go module before staging the commit.
- For terraform-test expansions: `cd simulators/<cloud>/terraform-tests && SOCKERLESS_TEST_TARGET=sim go test -run TestTerraformApplyDestroy` must PASS locally before the commit.
- For each new terraform resource: `curl -v` the sim handler with the canonical body the provider will send (capture via `TF_LOG=DEBUG` once) — verifies the wire shape independent of the provider's call sequence.
- `git log --oneline -1` after every commit to confirm SHA advanced (pattern 31).
- Re-verification pass against the diff after every sub-task closes (pattern 26 / 32).

## Resumable tracks after Phase 165 merges

### Track A — Live-cloud validation (carried from Phase 164)

Lambda live · Cloud Run Services + ACA Apps live · AZF cloud-dns live · Lambda service-mesh live · ACA/AZF Azure AD live. One branch per cell; teardown self-sufficient per `feedback_teardown_aggressive.md`.

### Track B — UI / TypeScript vibe-slop sweep (carried from Phase 161)

Sibling pattern check on `ui/packages/*/src/`. Open as Phase 166 if Phase 165 surfaces a parallel finding.

### Track C — Test-pyramid deepening (P1 follow-up to Phase 165)

After P165.5–7 close the P0 gaps, the remaining P1 surfaces (CloudFront full-distribution lifecycle, CloudWatch Metrics terraform, GCP Cloud Build terraform, Azure Application Insights terraform, GCP Operations terraform) can stage into a Phase 166 follow-up if leverage materialises.

### Track D — Phase 91d (bookmarked indefinitely)

Real `pd-ephemeral` on cloudrun + gcf. Cloud Run's `runpb.Volume` lacks a PD field. Don't reopen until cloud capability changes.

## Invariants snapshot (full list in STATUS.md + VIBE_CODING.md)

- Never auto-merge; user merges every PR.
- Components decoupled from admin / UI.
- No fakes / no fallbacks / no silent shims.
- Persistence opt-in + fail-loud on both open AND write.
- HTTP handlers dispatch through `s.self.<Method>`; never read `s.Store` directly.
- No phase / BUG-ID references in code comments (BUG-994 — extends to test docstrings per BUG-1036).
- `gh` CLI is the reference adaptor for bleephub.
- `aws --debug` + SDK serializer source are the reference for sim handler wire shapes.
- Terraform provider call sequences are materially different from raw SDK — both layers must be exercised (BUG-1029 lineage; ground for the P165.5–7 expansion).
- `specs/CLOUD_RESOURCE_MAPPING.md` is authoritative for cloud-mapping.

## Session-resume checklist

1. `git fetch origin && git checkout phase-165-vibe-slop-sweep-3-test-pyramid && git pull` (or `git checkout main && git pull --ff-only` if 165 merged).
2. `git log --oneline -15` to see what's already on the branch.
3. Read STATUS.md + this file + the last 2 commits + BUGS.md § Open.
4. Read [`.claude/skills/avoid-vibe-slop/SKILL.md`](.claude/skills/avoid-vibe-slop/SKILL.md) before writing any fix.
5. Pick the next `◻` row from the sub-task table above (severity-ordered).
6. Fix it. `go test ./...` in the touched module. Move the BUG from Open → Resolved history in BUGS.md with a one-line summary.
7. State save: update this file's sub-task status, STATUS.md bug counts, WHAT_WE_DID.md narrative.
8. Commit and push. CI green per push.
