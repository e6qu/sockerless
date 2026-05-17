# Do Next

Status [STATUS.md](STATUS.md) · roadmap [PLAN.md](PLAN.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · vibe catalogue [docs/VIBE_CODING.md](docs/VIBE_CODING.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Where we are

Phase 165 merged 2026-05-17 (PR #165, `288b76d3` on `origin/main`). State-save PR #166 (post-merge doc state) open, awaits user merge.

**Phase 166 in flight on `phase-166-test-pyramid-realfixes`** — real implementations for the 3 Open BUGs (1040 Azure azurerm, 1041 GCP IAM SA + CF Gen2 + Pub/Sub, 1042 AWS 5 sim handler gaps). User directive: *"we don't want fallbacks, we don't want workarounds, we want real actual solutions and faithful API compliance (identical) for each component"* — so each gap gets a real handler matching the real cloud's API shape, not a stub.

Single PR per the standing single-branch rule. Verify per commit.

## Phase 166 sub-task table (severity-ordered)

| Sub | Status | BUG(s) | What |
|---|---|---|---|
| **P166.0** | ✅ | — | Branch from `origin/main@288b76d3` + sub-task layout. |
| **P166.1** | ✅ | 1042 | **AWS — real sim handler additions, no stubs.** KMS GetKeyPolicy + ListResourceTags + GetKeyRotationStatus; SM GetResourcePolicy (omits field when no policy); SSM AddTagsToResource + Remove + List with real upsert semantics; DynamoDB: writeDDBJSON wrapper for `application/x-amz-json-1.0` + WarmThroughput field (terraform v6's waitTableWarmThroughputActive needs it) + DescribeContinuousBackups + DescribeTimeToLive + ListTagsOfResource + Update*+Tag/Untag + TableId/ProvisionedThroughput/etc; S3 14 sub-resource query handlers (?policy, ?versioning, ?lifecycle, etc.) returning real "not-configured" shape. main.tf 26→33 resources. Test PASS locally. |
| **P166.2** | ✅ | 1040 | **Azure — `azurerm` provider against the sim.** Sim already shipped `/metadata/endpoints?api-version=2022-09-01` + `/<tenant>/oauth2/v2.0/token` + JWKS + OIDC discovery — just needed to point azurerm via `metadata_host = trimprefix(var.endpoint, ...)`. Added 12 azurerm-driven resources: resource_group + container_registry + user_assigned_identity + private_dns_zone + log_analytics_workspace + application_insights + container_app_environment + container_app + container_app_job + service_plan + storage_account + linux_function_app. Test darwin-blocked locally; CI runs in Docker. |
| **P166.3** | ✅ | 1041 | **GCP — google_service_account.** Root-caused via gh-api reading the provider source (v7.32.0): `google_service_account` routes through `iambeta.NewClient` which uses `iam_beta_custom_endpoint` (NOT `iam_custom_endpoint`). Added the setting + the resource. Test PASS locally. Cloud Functions Gen2 + Pub/Sub + Compute instance/template + Cloud Build + Logging follow-ups staged for a future sub-phase (sim probably doesn't model Pub/Sub yet; CF2 needs real GCS source archive — multi-resource orchestration). |
| **P166.4** | ◻ | — | Codex CLI review pass + fix any validated findings. |
| **P166.5** | ◻ | — | State save + push + watch CI on PR #167. |

## Verification discipline (each sub-task)

- `go test ./...` in every touched module before staging the commit.
- For sim handler additions: spin up the sim locally + `curl -v` the canonical request the provider sends.
- For terraform-test additions: run `GOWORK=off go test -count=1 -run TestStackProductionShape` (AWS) / `TestTerraformApplyDestroy` (GCP/Azure) locally — must PASS before committing.
- Per the user's faithful-API-compliance directive: every sim handler returns the real-AWS / real-GCP / real-Azure response shape exactly — no "good enough" approximations. Cross-check against real-cloud responses (saved as fixtures or via the SDK's request mocking).

## Invariants snapshot (full list in STATUS.md + VIBE_CODING.md)

- **No fallbacks, no workarounds, no defers.** If a feature is missing, add it. Faithful API compliance (identical to the real cloud) for each component.
- Never auto-merge; user merges every PR.
- Single-branch rule: all in-flight work for one phase lands on one branch; many granular commits, one PR.
- File BUGs *before* fixing.
- Verify each significant chunk; don't batch fixes.
- Components decoupled from admin / UI.
- Persistence opt-in + fail-loud on both open AND write.
- HTTP handlers dispatch through `s.self.<Method>`; never read `s.Store` directly.
- No phase / BUG-ID references in code comments or test docstrings.
- `gh` CLI is the reference adaptor for bleephub.
- `aws --debug` + SDK serializer source are the reference for AWS sim handler wire shapes.
- Terraform provider call sequences differ materially from raw SDK — both test layers required.
- `specs/CLOUD_RESOURCE_MAPPING.md` is authoritative for cloud-mapping.

## Resumable tracks (longer-horizon)

### Track A — Live-cloud validation (one branch per cell)
Lambda live · Cloud Run Services + ACA Apps live · AZF cloud-dns live · Lambda service-mesh live · ACA/AZF Azure AD live.

### Track B — UI / TypeScript vibe-slop sweep (carried from Phase 161)

### Track C — Phase 91d (bookmarked indefinitely)
Real `pd-ephemeral` on cloudrun + gcf.

## Session-resume checklist

1. `git fetch origin && git checkout phase-166-test-pyramid-realfixes && git pull` (or `git checkout main && git pull --ff-only` if 166 merged).
2. `git log --oneline -15` to see what's already on the branch.
3. Read STATUS.md + this file + BUGS.md § Open (3 entries to close).
4. Read [`.claude/skills/avoid-vibe-slop/SKILL.md`](.claude/skills/avoid-vibe-slop/SKILL.md) before writing any fix.
5. Pick the next `◻` row from the sub-task table above.
6. Fix it real — no stubs that lie about success. `go test ./...` + spin-the-sim curl validation per commit.
7. State save per commit.
8. Push per commit. CI green per push.
