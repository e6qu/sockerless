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
| **P165.1** | ◻ | 1033 | `backends/core/{build.go:515,handle_images.go:80/115/472/493}` — 5 silent `io.Copy(w, rc)` swallows on image-stream + build response paths. Capture err + `s.logger.Debug(...)` (Debug because the connection is already gone; mirrors the BUG-1025 pktline pattern). Cross-cloud sweep: confirm sibling backend handlers (ECS/Lambda/CloudRun/etc. fs/stats/reverse-agent adapt layers) already check `_, err = io.Copy(...)`. |
| **P165.2** | ◻ | 1034 | `backends/lambda/agent_e2e_integration_test.go:168` — drop the `var _ = fmt.Sprintf // fmt is used by the framing demuxer when debugging` silencer + the `fmt` import. Re-grep the test file for any other false-claim silencers. |
| **P165.3** | ◻ | 1035 | Standardise on `_, _ = w.Write(...)` form at the three outlier sites (`bleephub/gh_oauth.go:171`, `bleephub/artifacts.go:321`, `simulators/azure/functions.go:290`). Add a single-word comment naming the client-disconnect-only failure mode where the surrounding pattern is silent. |
| **P165.4** | ◻ | 1036 | Sweep ~50 test-file docstrings carrying `// Phase NNN (PNNN.x) — …` / `// BUG-NNN` lineage. Rewrite each header to describe the **contract under test** (e.g. "issue / PR locking moderation enforces 403 on locked-parent comment-create"), not the phase that introduced it. Use `rg` to bound the sweep; preserve `// Phase 1: …` style runner-lifecycle phase labels in `tests/gitlab_runner_e2e_test.go` (those are docker-compose lifecycle phase numbers, not project phases — verified). |
| **P165.5** | ◻ | 1039 | **Azure terraform-tests expansion** — biggest cloud gap. Add `azurestack_storage_account` (Azure Files), `azurestack_key_vault` + key, `azurestack_container_registry`, `azurestack_container_app_environment` + `azurestack_container_app` + `azurestack_container_app_job`, `azurestack_function_app` + service plan, `azurestack_application_insights`, `azurestack_user_assigned_identity`, `azurestack_private_dns_zone` + record. Pre-validate each PUT against the running sim via curl (matches Phase 164.11 discipline). Surface any missing handlers as own-BUGs filed in this PR. |
| **P165.6** | ◻ | 1038 | **GCP terraform-tests expansion** — add `google_cloudfunctions2_function` (the runner-workload primitive!), `google_service_account` + `_iam_binding` + `_iam_member`, `google_storage_bucket_object`, `google_compute_subnetwork` + `_firewall` + `_instance` + `_instance_template`, `google_cloudbuild_trigger`, `google_logging_project_sink` + `_metric`, `google_pubsub_topic` + `_subscription`. Same discipline: surface missing handlers as new BUGs in this PR. |
| **P165.7** | ◻ | 1037 | **AWS terraform-tests expansion** — add `aws_lambda_function`, `aws_s3_bucket` + `_object`, `aws_dynamodb_table`, `aws_kms_key` + `_alias`, `aws_secretsmanager_secret` + `_version`, `aws_efs_file_system` + `_access_point` + `_mount_target`, `aws_ssm_parameter`, `aws_vpc` + `_subnet` + `_security_group` + `_security_group_rule`. Same discipline: pre-validate, surface gap-BUGs. |
| **P165.8** | ◻ | — | **Continuity-doc compression** — STATUS / DO_NEXT / PLAN / WHAT_WE_DID / README. Goal: actionable-across-compaction shape. Keep invariants block + active-phase scope + last 3 phase headlines + forward-looking tracks. Drop: closed-phase per-sub-task tables, per-BUG narrative paragraphs (covered by BUGS.md), duplicated "what's a vibe-slop sweep" prose. Target: ≤ ~50% current line count. Re-verify continuity by re-reading each doc as a cold-start operator would. |
| **P165.9** | ◻ | — | Final state save: STATUS / DO_NEXT / WHAT_WE_DID / PLAN / MEMORY updated to reflect every closed sub-task. Phase 165 narrative entry in WHAT_WE_DID. |
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
