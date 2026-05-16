# Sockerless — Status

Roadmap [PLAN.md](PLAN.md) · resume [DO_NEXT.md](DO_NEXT.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Snapshot

| | |
|---|---|
| Active branch | `phase-160-skills-from-159-lessons` — open as PR #160. |
| In-flight | **Phase 160 — Skills + complete docs sweep.** Two new project-local skills (`sim-handler-checklist`, `cross-resource-stack-test`) + `adaptor-fidelity-check` refinement (steps 1a + 1b) capturing the four recurring Phase 159 lessons. **PLUS** every component README now follows the Phase 157 adaptor-led shape: 6 cloud-backend READMEs + 2 simulator READMEs rewritten + bleephub Reference-adaptor section + `cmd/sockerless` rewrite + `cmd/sockerless-admin/README.md` (new) + `simulators/README.md` (rewritten as end-to-end showcase + navigation hub). External spec hyperlinks (Docker REST API, AWS/GCP/Azure SDK + CLI, Terraform provider registry, GitHub REST/GraphQL) throughout. Phase 157 Track A officially closed. |
| Last merged | PR #159 — Phase 159 AWS sim CloudFront/ACM/Route 53/WAFv2/Amplify/IAM SLR+OIDC (2026-05-15, merged at `236a387f`). |
| Standing merge auth | **None.** Default "never auto-merge" rule active. User merges every PR. |
| Cells | 8/8 runner-integration cells GREEN since 2026-05-07. |
| Bugs | 0 open · 992 fixed. |
| Live infra | None up. |

## Invariants (carry across compactions / fresh sessions)

### Process

- **Never auto-merge PRs.** Push, wait for `gh pr checks` green, ping user. One-time exceptions don't carry forward.
- **Single-branch rule.** All in-flight work for one phase lands on one branch; many granular commits, one PR.
- **State save every task.** STATUS.md + DO_NEXT.md + WHAT_WE_DID.md + MEMORY.md + this file's `_tasks/done/`.
- **Test all the time.** `go test ./...` in every touched module; harness-touch re-runs the harness; terraform-touch runs `terragrunt validate`.
- **Branch hygiene.** Rebase phase branch on `origin/main` before pushing; sync local `main` after merge.
- **Pre-push hooks own the truth.** If the `check-latest-deps` hook flags dep drift, bump deps in the same branch — never skip the hook.

### Architecture

- **Components stay decoupled from admin / UI.** Sims, backends, bleephub run independently via env vars; admin reads only `/v1/health`, `/v1/info`, env. No admin-required env vars on components, no startup registration.
- **Backend ↔ host primitive must match.** ECS in ECS, Lambda in Lambda, Cloud Run in Cloud Run, GCF in CRF, ACA in ACA, AZF in AZF.
- **No fakes / no fallbacks.** Unknown values fail loud. Operator-requested persistence + auth never silently degrade.
- **Persistence is opt-in + fail-loud.** `BLEEPHUB_PERSIST=true` / `SIM_PERSIST=true` → SQLite. Open-failure `log.Fatalf` (BUG-985/986 pattern); never silent in-memory fallback.
- **Test target gating.** Backend integration tests require `SOCKERLESS_TEST_TARGET=sim|cloud`; never implicit skip.
- **specs/CLOUD_RESOURCE_MAPPING.md is authoritative** for "how does sockerless model X on cloud Y."

### bleephub-specific (closed in Phases 153–156)

- **`gh` CLI is the reference adaptor.** If it works against `api.github.com`, it must work against bleephub. Test harness uses native `gh repo create / view / list`, `gh issue create / view / list / close / reopen`, `gh pr create / view / list / review / merge`, `gh release create`. No `gh api` URL hackery for happy path.
- **`gh` is HTTPS-only against non-`github.com` hosts.** Quick-start in `bleephub/README.md` covers the self-signed-cert + system-trust path; `host:port` in `--hostname` works if you can't bind `:443`.
- **GitHub Apps and OAuth Apps are separate concepts.** Distinct store entries, distinct token prefixes (`ghp_`/`gho_`/`ghu_`/`ghs_`/`ghr_`/`bph_`).
- **Installation tokens are immutable snapshots.** Re-mint to pick up perm changes.
- **Body coercion is per-GitHub-spec.** `flexBool` / `flexInt` / `flexInt64` / `flexIntSlice` accept both typed and string-coerced JSON (what `gh api -f` sends). Not a fallback; this is the GitHub Rails-layer behavior made explicit.

## Phase 159 — AWS simulator: CloudFront + Amplify + supporting IAM/Route 53/WAFv2/ACM (in flight)

Sockerless's `simulators/aws/` today implements ECS, ECR, IAM, EC2, EFS, Lambda, KMS, SSM, S3, STS, SecretsManager, DynamoDB, CloudWatch, CloudMap, Lambda Runtime API, metadata services. Phase 159 adds the front-of-house CDN/website-hosting surface: **CloudFront** and **AWS Amplify** plus the four supporting services those two consume:

- **CloudFront** — REST + XML wire (not JSON). Distributions, origin access controls (OAC) + legacy origin access identities (OAI), cache policies, origin request policies, response headers policies, behaviors, invalidations, CloudFront Functions, key groups + public keys, real-time log configs, monitoring subscriptions, tag CRUD, `AssociateAlias` / `ListConflictingAliases`.
- **AWS Amplify** — JSON wire. Apps, branches, domain associations, webhooks, deployments, jobs, backend environments, build artifacts, custom rules (redirects/rewrites), tag CRUD.
- **WAFv2** — JSON wire, scope-aware (`CLOUDFRONT` vs `REGIONAL`). WebACLs, rule groups, IP sets, regex pattern sets, association with CloudFront distributions, sampled-requests stub.
- **ACM** — JSON wire. `RequestCertificate` / `DescribeCertificate` / `DeleteCertificate` / `ListCertificates` / tag CRUD / `ImportCertificate`. us-east-1 region pinning enforced for CloudFront-associated certs.
- **Route 53 ALIAS** — extend the existing DNS handling (if any) or add it: hosted zones + record sets including `AliasTarget` records pointing at CloudFront distribution domain names + Amplify default domains.
- **IAM additions** — service-linked roles (`AWSServiceRoleForCloudFrontLogger`, `AWSServiceRoleForAmplify`), OIDC providers (for Amplify SSR identity), policies that already-existing IAM handlers may not synthesise yet.

Reference adaptors (per Phase 157 frame): `aws cloudfront`, `aws amplify`, `aws wafv2`, `aws acm`, `aws route53` CLI verbs + AWS Go SDK + Terraform `aws` provider resources (`aws_cloudfront_distribution`, `aws_cloudfront_function`, `aws_cloudfront_origin_access_control`, `aws_amplify_app`, `aws_amplify_branch`, `aws_amplify_domain_association`, `aws_wafv2_web_acl`, `aws_wafv2_web_acl_association`, `aws_acm_certificate`, `aws_route53_record` with `alias{…}`).

Sub-task breakdown lives in [DO_NEXT.md § Phase 159](DO_NEXT.md). State save + phase scoping is sub-task 0; implementation sub-tasks land as separate commits on this branch.

## Recently closed phases

| PR | Phase | Headline |
|---|---|---|
| #158 | 158 | BUG-991 + BUG-992 fixes (handler→`s.self` delegation closed two fallback-hiding-bugs in `handleContainerWait` + `handleImageList`). `docs/VIBE_CODING.md` 23-pattern catalogue. `docs/GOLANG_STRONG_TYPING.md` 15-approach research-only catalogue. Three project-local Claude skills under `.claude/skills/`. |
| #157 | 157 | Component ⇄ reference-adaptor docs sweep + state save + experimental/security caveat on root README + `backends/docker/README.md` rewrite. BUG-991 surfaced. |
| #156 | 156 | Project-wide docs refresh + bleephub Quick start + `gh` CLI `--hostname` clarification + GCP dep bump. |
| #155 | 155 | bleephub-specific docs refresh — `bleephub/README.md`, `docs/BLEEPHUB_GH_CLI.md`, `specs/BLEEPHUB_GITHUB_API_PARITY.md`, `ARCHITECTURE.md`. |
| #154 | 154 | Broad GitHub API sweep — reactions, releases, deployments + environments, PR review comments + threads, Checks, Actions OIDC + JWKS, Pages, branch protection. Real `gh` CLI Docker harness 50/50 PASS. |
| #153 | 153 | bleephub ↔ GitHub API parity + SQLite persistence + real `gh` CLI compat (13 sub-tasks). |
| #152 | docs | `docs/POD_MATERIALIZATION.md` — per-backend pod materialization walked through GH + GitLab runners. |
| #151 | 87d + 92 | Trace propagation + MeterProvider + runtime metrics + `make stack-observability-validate`; `Backing: gcs-fuse` deregistered on cloudrun + gcf. |
| #150 | 87c | zerolog → OTel logs bridge across all 12 components. |
| #149 | 91 | Lambda volume_translator framework migration; cloudrun + gcf reject `BackingPDEphemeral`. |

Older PRs (#112–#148) headline-summarised in [PLAN.md § Closed phases](PLAN.md). Narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md).
