# Do Next

Status [STATUS.md](STATUS.md) · roadmap [PLAN.md](PLAN.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · vibe catalogue [docs/VIBE_CODING.md](docs/VIBE_CODING.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Where we are

Phase 165 merged 2026-05-17 (PR #165, `288b76d3` on `origin/main`). State-save PR #166 (post-merge doc state) open, awaits user merge.

**Phase 166 in flight on `phase-166-test-pyramid-realfixes`** — real implementations for the 3 Open BUGs (1040 Azure azurerm, 1041 GCP IAM SA + CF Gen2 + Pub/Sub, 1042 AWS 5 sim handler gaps). User directive: *"we don't want fallbacks, we don't want workarounds, we want real actual solutions and faithful API compliance (identical) for each component"* — so each gap gets a real handler matching the real cloud's API shape, not a stub.

Single PR per the standing single-branch rule. Verify per commit.

## Phase 166 sub-task table (severity-ordered: 1042 P0 first, then 1040 widest, then 1041)

| Sub | Status | BUG(s) | What |
|---|---|---|---|
| **P166.0** | ✅ | — | Branch from `origin/main@288b76d3` + sub-task layout. |
| **P166.1** | ◻ | 1042 | **AWS — real sim handler additions, no stubs.** Implement: (a) S3 path-style support via `s3_use_path_style = true` in terraform provider config + verify sim routes match (`/s3/{bucket}` already mounts correctly when the provider sends path-style); (b) KMS `TrentService.GetKeyPolicy` returning real-AWS default key policy (root-account-allows-all IAM doc); (c) Secrets Manager `secretsmanager.GetResourcePolicy` returning real `{ARN, Name, ResourcePolicy}` shape (empty ResourcePolicy when none set, matching real AWS); (d) SSM `AmazonSSM.ListTagsForResource` returning real `{TagList: [...]}` shape (empty list when none set); (e) DynamoDB DescribeTable response shape audit — add `TableId` (UUIDv4) + `TableThroughputMode` + `DeletionProtectionEnabled` + `ProvisionedThroughput` zero-fill for PAY_PER_REQUEST (terraform polls these). Then add `aws_s3_bucket` + `aws_dynamodb_table` + `aws_kms_key/_alias` + `aws_secretsmanager_secret/_version` + `aws_ssm_parameter` to terraform-tests/main.tf with cross-resource assertions. |
| **P166.2** | ◻ | 1040 | **Azure — `azurerm` provider against the sim.** Research path: real-cloud config via `cloud { metadata_host = ... }` block + auth via `oidc_request_token_file` or `client_id`/`client_secret`. May need sim-side endpoints: `/metadata/endpoints?api-version=2020-06-01` (cloud-discovery), `/{tenant}/oauth2/v2.0/token` (token mint), `/.well-known/openid-configuration` (JWKS). Once `azurerm` reaches the sim, add: `azurerm_container_registry`, `azurerm_container_app_environment` + `azurerm_container_app` + `azurerm_container_app_job`, `azurerm_linux_function_app` + `azurerm_service_plan`, `azurerm_application_insights`, `azurerm_user_assigned_identity`, `azurerm_private_dns_zone` + `_a_record`, `azurerm_key_vault_key` + `_secret`. Each new resource = surface any missing sim handlers as new BUGs + fix in this PR. |
| **P166.3** | ◻ | 1041 | **GCP — IAM SA + Cloud Functions Gen2 + Pub/Sub.** (a) `google_service_account` — terraform-provider-google's IAM resources hard-code `iam.googleapis.com`; fix path: bind sim to that hostname via SOCKERLESS_DNS_OVERRIDE or add a reverse-proxy in TestMain mapping `iam.googleapis.com → sim`. (b) `google_cloudfunctions2_function` — add `google_storage_bucket_object` source archive (already covered by Phase 165's GCS object), then reference it in build_config. (c) Pub/Sub — if sim doesn't model it, add `simulators/gcp/pubsub.go` with topic + subscription CRUD then `google_pubsub_topic` + `_subscription`. (d) `google_compute_instance` + `_instance_template` — likely needs sim instance handlers. (e) `google_cloudbuild_trigger`, `google_logging_project_sink` + `_metric`. Same discipline: surface gaps, fix in this PR. |
| **P166.4** | ◻ | — | Codex CLI review pass + fix any validated findings. |
| **P166.5** | ◻ | — | Continuity-doc compression (incremental, after each sub-task closes). |
| **P166.6** | ◻ | — | Final state save + open PR + watch CI. |

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
