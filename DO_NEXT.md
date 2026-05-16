# Do Next

Status [STATUS.md](STATUS.md) · roadmap [PLAN.md](PLAN.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md) · vibe-coding catalogue [docs/VIBE_CODING.md](docs/VIBE_CODING.md) · typing research [docs/GOLANG_STRONG_TYPING.md](docs/GOLANG_STRONG_TYPING.md).

## Where we are

Phase 159 merged 2026-05-15 (PR #159, commit `236a387f` on origin/main).

Phase 160 in flight on `phase-160-skills-from-159-lessons`: codify the four load-bearing Phase 159 lessons into new project-local Claude skills and refine `adaptor-fidelity-check`. No code-surface changes — `.claude/skills/`, `docs/VIBE_CODING.md`, and continuity docs only.

P160 acceptance:
- New skill `sim-handler-checklist` covers SDK serializer source, TF provider `resourceXxxRead` inspection, asymmetric Create/Read APIs, trailing-slash routing.
- New skill `cross-resource-stack-test` codifies the `TestStackProductionShape` pattern.
- `adaptor-fidelity-check` gains steps 1a (SDK serializer) and 1b (TF provider Read inspection).
- `docs/VIBE_CODING.md` project-local-skills section reflects all five skills.

Default "user merges every PR" remains in force.

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
| **P159.0** | ✅ | State save + dep tidy (aca/azf go.sum after azure-common bump). |
| **P159.1** | ✅ | CloudFront `Distribution` + `OriginAccessControl` + Tagging — first XML-bodied service. Wire pattern + cfNormalizeConfig + ETag/If-Match all locked in. PR commit `bf85f382`. |
| **P159.2** | ✅ | CloudFront `CachePolicy` + `OriginRequestPolicy` + `ResponseHeadersPolicy` — independent CRUDs, no inter-resource dependencies. PR commit `94331059`. |
| **P159.3** | ✅ | CloudFront `Function` (DEVELOPMENT→LIVE) + `Invalidation` (per-distribution) + `PublicKey` + `KeyGroup` (with PublicKey reference dependency). PR commit `fe2c6e81`. |
| **P159.4** | ✅ | ACM — us-east-1 pin enforcement + full CRUD + `DescribeCertificate` shape + `aws_acm_certificate` terraform test + sdk + cli tests. PR commit. |
| **P159.5** | ✅ | Route 53 — XML codec extension + zones + record sets + `AliasTarget` referencing CloudFront distribution + `aws_route53_record` (with `alias{…}`) terraform test + sdk + cli tests. PR commit. |
| **P159.6** | ✅ | WAFv2 — JSON, CLOUDFRONT scope; WebACLs + IPSets + RuleGroups + RegexPatternSets + Association + sdk + cli + terraform tests. ARN region is literally `us-east-1`, path includes `global/`. PR commit. |
| **P159.7** | ✅ | Amplify apps + branches + webhooks + jobs (synthesised SUCCEEDED) + sdk + cli + terraform tests. PR commit. |
| **P159.8** | ✅ | Amplify domains + custom rules + backend environments + sdk + cli + terraform tests. PR commit. |
| **P159.9** | ✅ | IAM extension — service-linked roles + OIDC providers + sdk + cli + terraform tests. Shadow `IAMRole` write so `aws_iam_service_linked_role` Read (which calls `GetRole`) converges. PR commit `3c95acf4`. |
| **P159.10** | ✅ | `simulators/aws/API_SPEC.md` §8–13 added covering every new verb (CloudFront, ACM, Route 53, WAFv2, Amplify, IAM extensions) + REST path reference appendix. `simulators/aws/README.md` rewritten in Phase 157 adaptor-led shape (reference adaptor / validation / wiring / sample / known issues / out-of-scope). End-to-end `TestStackProductionShape` provisions CloudFront + ACM + WAFv2 + Route 53 ALIAS + Amplify + IAM SLR/OIDC + ECS + Cloud Map in one apply, asserts WAF.resource_arn == CloudFront.arn, Route 53 ALIAS target == CloudFront domain_name, ACM ARN region == us-east-1. PR commit 2026-05-15. |

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

### Track A — Phase 157 component-adaptor sweep (CLOSED in Phase 160)

The 8 README rewrites that carried forward from Phase 157 all landed in PR #160:

| Component | Reference adaptor | Status |
|---|---|---|
| `backends/ecs` | docker SDK/CLI + aws CLI/SDK + Terraform aws | ✅ Phase 160 |
| `backends/lambda` | docker SDK/CLI + aws CLI/SDK + Terraform aws | ✅ Phase 160 |
| `backends/cloudrun` | docker SDK/CLI + gcloud + GCP SDK + Terraform google | ✅ Phase 160 |
| `backends/cloudrun-functions` | docker SDK/CLI + gcloud + GCP SDK + Terraform google | ✅ Phase 160 |
| `backends/aca` | docker SDK/CLI + az + Azure SDK + Terraform azurerm | ✅ Phase 160 |
| `backends/azure-functions` | docker SDK/CLI + az + Azure SDK + Terraform azurerm | ✅ Phase 160 |
| `simulators/gcp` | gcloud + GCP SDK + Terraform google | ✅ Phase 160 |
| `simulators/azure` | az + Azure SDK + Terraform azurerm | ✅ Phase 160 |
| `simulators/aws` | aws CLI + AWS SDK + Terraform aws | ✅ P159.10 |
| `backends/docker` | docker CLI/SDK + podman CLI | ✅ #157 |
| `bleephub` | gh CLI + actions/runner + smart-HTTP git + GitHub REST/GraphQL specs | ✅ Phase 160 (reference-adaptor section added) |

**Still un-rewritten (low priority, deferred):** `cmd/sockerless/README.md`, `cmd/sockerless-admin/README.md`, `simulators/README.md` end-to-end showcase. These can pick up a future small phase if needed.

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
