# Do Next

Status [STATUS.md](STATUS.md) · roadmap [PLAN.md](PLAN.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md) · vibe-coding catalogue [docs/VIBE_CODING.md](docs/VIBE_CODING.md) · typing research [docs/GOLANG_STRONG_TYPING.md](docs/GOLANG_STRONG_TYPING.md).

## Where we are

Phase 159 (AWS simulator: CloudFront + Amplify + supporting IAM/Route 53/WAFv2/ACM) starting on `phase-159-aws-sim-cloudfront-amplify`, off `origin/main` post-PR-#158 merge.

PR #158 merged 2026-05-13 with user-authorized one-time merge. Default "user merges every PR" is back.

## Phase 159 scope (locked-in)

Expand `simulators/aws/` to cover six service families currently absent from the simulator. **Reference adaptors** drive validation per Phase 157 frame: `aws` CLI verbs + AWS Go SDK + Terraform `aws` provider resources. See [PLAN.md § Phase 159](PLAN.md) for the protocol notes and out-of-scope.

### Service-by-service surface area

| File (new) | Service | Wire | Core verbs |
|---|---|---|---|
| `simulators/aws/cloudfront.go` | CloudFront | REST + XML | `CreateDistribution`, `GetDistribution`, `UpdateDistribution`, `DeleteDistribution`, `ListDistributions`, `CreateInvalidation`, `ListInvalidations`, `GetInvalidation`, `CreateOriginAccessControl`, `GetOriginAccessControl`, `ListOriginAccessControls`, `DeleteOriginAccessControl`, `CreateCachePolicy`, `GetCachePolicy`, `DeleteCachePolicy`, `ListCachePolicies`, `CreateOriginRequestPolicy`, `CreateResponseHeadersPolicy`, `CreateFunction`, `PublishFunction`, `DescribeFunction`, `ListFunctions`, `DeleteFunction`, `UpdateFunction`, `CreateKeyGroup`, `CreatePublicKey`, `AssociateAlias`, `ListConflictingAliases`, `CreateMonitoringSubscription`, tag CRUD. |
| `simulators/aws/amplify.go` | Amplify | JSON | `CreateApp`, `UpdateApp`, `DeleteApp`, `GetApp`, `ListApps`, `CreateBranch`, `GetBranch`, `ListBranches`, `UpdateBranch`, `DeleteBranch`, `CreateDomainAssociation`, `GetDomainAssociation`, `ListDomainAssociations`, `UpdateDomainAssociation`, `DeleteDomainAssociation`, `CreateWebhook`, `GetWebhook`, `ListWebhooks`, `UpdateWebhook`, `DeleteWebhook`, `CreateDeployment`, `StartDeployment`, `StartJob`, `GetJob`, `ListJobs`, `StopJob`, `CreateBackendEnvironment`, `GetBackendEnvironment`, `ListBackendEnvironments`, `DeleteBackendEnvironment`, tag CRUD, `GenerateAccessLogs`, `GetArtifactUrl`. |
| `simulators/aws/wafv2.go` | WAFv2 (CLOUDFRONT scope) | JSON | `CreateWebACL`, `GetWebACL`, `UpdateWebACL`, `DeleteWebACL`, `ListWebACLs`, `AssociateWebACL`, `DisassociateWebACL`, `GetWebACLForResource`, `ListResourcesForWebACL`, `CreateRuleGroup`, `GetRuleGroup`, `UpdateRuleGroup`, `DeleteRuleGroup`, `ListRuleGroups`, `CreateIPSet`, `GetIPSet`, `UpdateIPSet`, `DeleteIPSet`, `ListIPSets`, `CreateRegexPatternSet`, tag CRUD, `GetSampledRequests` (stub). |
| `simulators/aws/acm.go` | ACM (us-east-1 pin) | JSON | `RequestCertificate`, `DescribeCertificate`, `DeleteCertificate`, `ListCertificates`, `AddTagsToCertificate`, `RemoveTagsFromCertificate`, `ListTagsForCertificate`, `ImportCertificate`, `ResendValidationEmail`, `UpdateCertificateOptions`. |
| `simulators/aws/route53.go` | Route 53 | REST + XML | `ListHostedZones`, `CreateHostedZone`, `GetHostedZone`, `DeleteHostedZone`, `ChangeResourceRecordSets` (incl. `AliasTarget` block), `ListResourceRecordSets`, `GetChange`, `CreateHealthCheck`, `GetHealthCheck`, `DeleteHealthCheck`, tag CRUD. |
| `simulators/aws/iam.go` (extend) | IAM service-linked roles + OIDC | JSON | Add `CreateServiceLinkedRole` for `AWSServiceRoleForCloudFrontLogger` + `AWSServiceRoleForAmplify`; `CreateOpenIDConnectProvider`, `GetOpenIDConnectProvider`, `DeleteOpenIDConnectProvider`, `ListOpenIDConnectProviders`. |

### Sub-task / commit layout

**Per-service-bundled tests**, not batched at the end. Each service sub-task lands with:

1. The handler file (`simulators/aws/<service>.go`)
2. SDK test (`simulators/aws/sdk-tests/<service>_test.go`) driving real AWS Go SDK
3. **Terraform test** (`simulators/aws/terraform-tests/<service>/`) driving the real Terraform `aws` provider with `endpoints {}` override — must apply, plan-no-drift, and destroy clean
4. CLI test (`simulators/aws/cli-tests/<service>_test.go`) driving real `aws` CLI

This satisfies the existing pre-commit hook "Simulator testing contract (SDK + CLI + terraform per change)" and gives every new sim handler three independent validations. Each sub-task = one commit; CI runs per push. Phase may span sessions — state save preserves continuity.

| Sub | Status | What (includes handler + SDK + Terraform + CLI tests) |
|---|---|---|
| **P159.0** | ✅ | State save — STATUS / PLAN / DO_NEXT / BUGS / WHAT_WE_DID locking in Phase 159 scope. |
| **P159.1** | pending | CloudFront skeleton — main.go route registration + XML codec shape + `Distribution`, `OriginAccessControl` CRUD + `aws_cloudfront_distribution` + `aws_cloudfront_origin_access_control` terraform tests + sdk + cli tests. |
| **P159.2** | pending | CloudFront `CachePolicy`, `OriginRequestPolicy`, `ResponseHeadersPolicy` + matching terraform (`aws_cloudfront_cache_policy`, etc.) / sdk / cli tests. |
| **P159.3** | pending | CloudFront Functions + invalidations + key groups + aliases + monitoring + tag CRUD + matching tests. |
| **P159.4** | pending | ACM — us-east-1 pin + full CRUD + `DescribeCertificate` shape + `aws_acm_certificate` terraform test (validation_method=DNS path) + sdk + cli tests. |
| **P159.5** | pending | Route 53 — XML codec extension + zones + record sets + `AliasTarget` referencing CloudFront distribution domain names + `aws_route53_record` (with `alias{…}`) terraform test + sdk + cli tests. |
| **P159.6** | pending | WAFv2 — JSON, CLOUDFRONT scope; WebACLs + IPSets + RuleGroups + AssociateWebACL with CloudFront ARN target + `aws_wafv2_web_acl` + `aws_wafv2_web_acl_association` terraform tests + sdk + cli tests. |
| **P159.7** | pending | Amplify apps + branches + webhooks + jobs (synthesised; no real build) + `aws_amplify_app` + `aws_amplify_branch` + `aws_amplify_webhook` terraform tests + sdk + cli tests. |
| **P159.8** | pending | Amplify domains + custom rules + backend environments + `aws_amplify_domain_association` terraform test + sdk + cli tests. |
| **P159.9** | pending | IAM extension — service-linked roles (`AWSServiceRoleForCloudFrontLogger`, `AWSServiceRoleForAmplify`) + OIDC providers + `aws_iam_service_linked_role` + `aws_iam_openid_connect_provider` terraform tests + sdk + cli tests. |
| **P159.10** | pending | `simulators/aws/API_SPEC.md` updated with every new verb covered. `simulators/aws/README.md` rewritten in Phase 157 adaptor-led shape. End-to-end terraform test plan that provisions CloudFront + ACM cert + WAF + Route 53 ALIAS together (the production "shape"). State save + close. |

The Terraform-test stance carries the same expectation as the user's direction: **expand terraform provider tests against the newly added functionality** — not a smoke per resource, but real `terraform apply` + plan-no-drift + destroy for each. Use the existing `simulators/aws/terraform-tests/` runner pattern (real Terraform binary, `endpoints {}` block overriding to the sim, `make terraform-tests` driving go-test wrappers).

### Discipline reminders

Before each sub-task commit, read `.claude/skills/avoid-vibe-slop/SKILL.md` checklist. Specifically:

- Q2 "What is the reference adaptor?" — `aws <service>` CLI + AWS Go SDK + Terraform `aws_*` resource. If you can't name it for this verb, you don't know if the change is right.
- Q3 "Does the adaptor's real behaviour confirm what I'm about to write?" — capture wire shape with `aws --debug <service> <verb>` against real AWS before guessing.
- Q4 "Is it a fallback or lying about success?" — sim must return real AWS error shapes (e.g., `NoSuchDistribution`, `InvalidParameterValue`) when state is wrong, not a synthesised 200.
- BUG-991/992 lineage especially relevant: **simulator handlers must consult their own store and return real errors for missing resources, not silent success.**

For wire-shape capture:

```bash
aws --debug --endpoint-url https://cloudfront.amazonaws.com cloudfront create-distribution --distribution-config file://config.json 2>&1 | grep -E "Body|URL|Method" | head -30
```

For Terraform-driven validation:

```bash
cd simulators/aws/terraform-tests/cloudfront
terraform plan -refresh=true  # endpoints { cloudfront = "http://localhost:5566/" }
```

### Acceptance bar

- Each new sim handler returns the real AWS wire shape (XML for CloudFront/Route 53; JSON for the others). Verified by running `aws <verb>` against the sim and getting parse-success on the SDK side.
- For each service, at least one `sdk-test` + one `terraform-test` + one `cli-test` lands green.
- `simulators/aws/API_SPEC.md` enumerates every new verb covered with last-green dates.
- `simulators/aws/README.md` (when written, P159.12) follows Phase 157 adaptor-led shape.
- No "synthesised success" patterns — missing resources return real error codes.

### Out of scope

- WAFv2 REGIONAL scope (ALB/API Gateway path) — deferred until a backend needs it.
- CloudFront Functions actually executing JavaScript — handlers store + return the code; do not interpret.
- Lambda@Edge runtime — association metadata only, no execution.
- Amplify real build pipeline — jobs return synthesised `SUCCEEDED` after a short pause; no npm/yarn.
- ACM DNS-validation polling realism — eager `ISSUED` transition (real AWS takes hours).
- Service-linked role enforcement — create on demand; do not gate operations on SLR existence.

## Resumable tracks after Phase 159 merges

### Track A — Resume Phase 157 component-adaptor sweep (deferred during 158 + 159)

Phase 157 PR #157 only covered `backends/docker`. Remaining: backends/{ecs,lambda,cloudrun,cloudrun-functions,aca,azure-functions}, simulators/{aws,gcp,azure}, `simulators/README.md` end-to-end showcase, cmd/sockerless, cmd/sockerless-admin. The `simulators/aws/README.md` portion likely folds into P159.12.

Component matrix:

| Component | Reference adaptor | Validation entry point |
|---|---|---|
| `backends/ecs` | aws CLI/SDK + Terraform aws; docker CLI | `simulators/aws/sdk-tests` + Docker SDK e2e |
| `backends/lambda` | aws CLI/SDK + Terraform aws; docker CLI | same |
| `backends/cloudrun` | gcloud + Go SDK + Terraform google; docker CLI | `simulators/gcp/sdk-tests` |
| `backends/cloudrun-functions` | gcloud + Go SDK + Terraform google; docker CLI | same |
| `backends/aca` | az + Go SDK + Terraform azurerm; docker CLI | `simulators/azure/sdk-tests` |
| `backends/azure-functions` | az + Go SDK + Terraform azurerm; docker CLI | same |
| `simulators/gcp` | gcloud + Go SDK + Terraform google | `simulators/gcp/{sdk-tests,terraform-tests}` |
| `simulators/azure` | az + Go SDK + Terraform azurerm | `simulators/azure/{sdk-tests,terraform-tests}` |
| `cmd/sockerless` (CLI) | itself — CLI is adaptor for backends | `cmd/sockerless/*_test.go` |
| `cmd/sockerless-admin` | browser / REST clients against `/v1/*` | `cmd/sockerless-admin/*_test.go` |

### Track B — Skill maturation (post-Phase 158)

Candidate additional skills as new patterns surface:

- `state-save` — codify the STATUS/PLAN/DO_NEXT/BUGS/WHAT_WE_DID refresh rhythm.
- `spec-first-implementation` — verify spec exists in `specs/` before coding.
- `cross-cloud-sweep` — formal procedure for the "if found in one backend, check the other 5" rule.
- `simulator-handler` — repeatable pattern for a new sim service file (XML codec, JSON codec, store type, error shapes).

### Track C — Live-cloud validation

Lambda live · Cloud Run Services + ACA Apps live · AZF cloud-dns live · Lambda service-mesh live · ACA/AZF Azure AD live. One branch per cell.

### Track D — Phase 91d (bookmarked indefinitely)

Real `pd-ephemeral` on cloudrun + gcf. Don't reopen until cloud capability changes.

## Invariants snapshot (full list in STATUS.md + VIBE_CODING.md)

- Never auto-merge; user merges every PR.
- Components decoupled from admin / UI.
- No fakes / no fallbacks / no silent shims.
- Backend ↔ host primitive must match.
- Simulator returns real AWS error shapes on missing/invalid state — never synthesised 200.
- `gh` CLI is the reference adaptor for bleephub; HTTPS-only, `--hostname` is the wiring flag.
- `aws --debug` is the reference for sim handler wire shapes — capture before writing.
- `specs/CLOUD_RESOURCE_MAPPING.md` is authoritative for cloud-mapping.
- Read `docs/VIBE_CODING.md` + `.claude/skills/avoid-vibe-slop/SKILL.md` checklist before any non-trivial code change.

## Session-resume checklist

1. `git fetch origin && git checkout phase-159-aws-sim-cloudfront-amplify && git pull` (or `git checkout main && git pull --ff-only` if 159 merged).
2. `git log --oneline -15` to see what's already on the branch.
3. Read STATUS.md + this file + the recent commits.
4. Read `.claude/skills/avoid-vibe-slop/SKILL.md` checklist before writing handler code.
5. Read `.claude/skills/adaptor-fidelity-check/SKILL.md` before writing wire-shape code; capture real AWS wire via `aws --debug` first.
6. Manual test before claiming a sub-task done (per `.claude/skills/manual-test/SKILL.md`); for sim sub-tasks, the recipe is "start sim, run real `aws` CLI / SDK / Terraform against it, paste real captured output."
7. File BUGS.md entries for anything that surfaces; fix in the same session.
8. State-save before pushing each sub-task commit; update the P159.X status row in this file.
