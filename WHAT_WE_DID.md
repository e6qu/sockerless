# Sockerless - What We Built

Roadmap [PLAN.md](PLAN.md) - status [STATUS.md](STATUS.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

Detailed historical narrative lives in PR descriptions and `git log`. This file kept the recent chain and a compact foundation summary.

## 2026-06-12 - Amplify full support + bleephub GitHub Apps/orgs hardening

**Amplify is now a complete service slice**: beyond the control-plane gap
pass (real deployment upload flow with byte-verified PUTs into sim S3,
pagination, cascades, presence-based updates, typed shapes, observable job
states — BUG-1717..1723), the sim gained the full execution layer per the
sim host model: REAL builds (buildSpec phases in node containers off the
ECR Public mirror, go-git clones, per-step logs served from resolvable
logUrls, honest SUCCEED/FAILED from container exit, StopJob cancels the
container), a Host-routed hosting data plane ({branch}.{appId}.amplifyapp
.com + deterministic cloudfront.net hosts + verified custom domains;
CustomRules rewrite/redirect/SPA semantics; basic auth), SSR/WEB_COMPUTE
via the published deploy-manifest spec (lazy long-lived compute containers
reverse-proxied per manifest routes; PublishPorts/WorkingDir added to the
shared container layer), and a REAL Route 53 verification state machine
for domain associations (read-evaluated against sim hosted zones; the
terraform fixture creates the verification records like real configs).
Also: a pre-existing P1 — every UI-embedded AWS sim binary panicked at
startup on a mux conflict CI never saw (noui fallback) — BUG-1715.

**bleephub apps/orgs** (BUG-1706..1714): installation-token downscoping
was entirely unenforced (P1) and suspension didn't block existing tokens
(P1) — both fixed with real 422/403 semantics; JWT exp-window fidelity
(backdated-iat clients were wrongly rejected); 7 new endpoint families
(manifest form flow, org installations/global list, public members,
user-side membership accept, team memberships/repos/children, full org
webhooks with repo-event fan-out); typed role/state/privacy enums; test
pyramid from JWT unit tables through go-github flows to native gh e2e
(harness 99 PASS / 0 FAIL).

## 2026-06-11 - Launch hygiene: validation armed everywhere + azurestack retired + docs truth pass

Closing the validation arc's last gaps in one PR. The terraform CI jobs now
run with `SOCKERLESS_SPEC_VALIDATE` armed + per-cloud ratchet steps — and
the very first armed terraform traffic caught BUG-1702 (SFN
DescribeStateMachine emitting `tags`, a path sdk/cli never drove). The AWS
runtime validator gained the XML protocols (awsQuery/ec2Query envelopes,
flattened/ec2QueryName lists, restXml roots with payload/header binding
exclusions), instantly catching BUG-1700 (EC2 `vCpuInfo` casing) and
BUG-1701 (four wrong S3 list-configuration XML root names) that lenient SDK
decoders had masked. BUG-1584 got its real fix: the azure terraform suite
migrated entirely off the AzureStack compatibility provider (its 7 resources
to azurerm equivalents, warning proven gone, coverage equal-or-better).
BUG-1104 (P0 audit-cadence meta) closed — the cadence is structural now.
Plus: `build-azf-bootstrap` (BUG-1703), the StatusBadge `waiting` token, and
a full documentation accuracy sweep (root README, FEATURE_MATRIX, bleephub
README's persistence/ratchet/approvals story, MAKEFILE_STANDARD, surface-
table index regenerated, stale claims across ~10 files corrected).

## 2026-06-11 - Simulator shape-drift burn-down (28 bugs, allowlists emptied)

All 28 runtime spec-shape bugs from the validation arc fixed in one PR
(BUG-1658..1685): aws + azure `spec-violation-allowlist.txt` deleted, gcp
reduced to its two permanent firestore server-streaming exemptions. The
recurring shapes: invented members removed, list-vs-describe context splits
(bespoke summary projections), sockerless wiring (`dockerNetworkName`,
`simImage`/`simCommand`) taken OFF the wire while staying persisted and
load-bearing (persisted-DTO embeds with unchanged row JSON / MarshalJSON
wire views — the bleephub `json:"-"` persistence trap explicitly avoided
and regression-tested), and wire-name fidelity (KV cert policy typed to the
swagger member set; public-vs-private DNS genuinely differ on `minimumTTL`
casing). Structural items: the gcp Cloud Run knative surface moved to the
real `/apis/serving.knative.dev/v1/...` paths with a canonical run/v1
client round-trip test (backends were already on run/v2), and azure
postgres-flexible now speaks the spec's 202-only LRO with
Azure-AsyncOperation polling. Several latent leaks beyond the filed bugs
died on the way, and two sim-quirk CLI test bodies (raw non-wire shapes no
real client sends) were corrected. All armed SDK+CLI suites green at zero
across the three clouds; cloudrun/gcf/aca FaaS smokes green.

## 2026-06-11 - bleephub deep sweep (shape ratchet, approvals, persistence, gh-CLI parity)

One fat PR sweeping bleephub's API, implementation, storage, and UI, anchored
by a new regression gate: every 2xx `/api/v3` JSON response flowing through
the shared test server is validated member-by-member against the vendored
GitHub OpenAPI description, ratcheted via `openapi-violation-allowlist.txt`
(one cited GHES-divergence entry; everything else fixed). First armed run
found 723 violations across 14 entity emitters — all fixed (simple/full
user/org/team splits, full repository shape, pull-request simple/full,
hypermedia templates, real counters).

- **Deployment approvals** (BUG-1590): environments parse reviewers/wait_timer
  and emit protection_rules; jobs targeting reviewer-protected environments
  hold in `waiting`; `GET/POST .../pending_deployments` + real `approvals`.
- **Webhook org block** (BUG-1618): central attach in `emitWebhookEvent`.
- **Persistence** (BUG-1692..1695): the `json:"-"`-stripping class destroyed
  app credentials/webhook secrets/secret values/linkage on reload; revoked
  tokens resurrected (P1); mutations skipping persistence; fail-loud on
  persistence-with-memory-git; DeleteRepo cascade; a reload test per gap.
- **gh-CLI parity** (BUG-1696..1699): GraphQL drift had silently broken
  `repo clone`, the whole `pr` chain, `release list/view/delete`; plus
  missing `/api/v3/meta` and push-run workflow_id derivation from an empty
  repo name. Fixed against verbatim gh v2.92 queries; the Docker harness now
  drives every previously-broken command natively: 92 PASS / 0 FAIL.
- **UI** (BUG-1690/1691): spinner dead-ends, login loop, Link-aware
  pagination with honest "N+" badges, draft-release rendering, type truthing;
  49 new vitest cases.

## 2026-06-11 - Spec-based simulator validation (specs/cloud-api + two gates)

The simulators are now validated against the official machine-readable cloud
API specs, so fidelity cannot silently diverge from what real clients are
generated from. Vendored under `specs/cloud-api/` (gzipped, never edited,
pinned, provenance in per-cloud `SOURCES.md`, fetch + freshness scripts):
37 AWS Smithy models (aws-sdk-go-v2 aws-models @ one SHA), 27 GCP Discovery
documents (pinned by revision), 112 Azure Swagger files (azure-rest-api-specs
@ one SHA; api-versions follow the pinned canonical clients).

Two enforcement layers, both hermetic:

- **Static surface conformance** — the shared sim lib records every
  registered pattern (`Server.Handle/HandleFunc` + AWS router accessors);
  `buildSimulator()` extracted from each cloud's `main()` lets
  `spec_conformance_test.go` build the full operation table in-process and
  assert every operation exists in the vendored spec (justified in-test
  allowlists for IMDS / OCI data planes / LRO polling URLs / `/sim/v1`).
  Runs in `make unit-test`; hard CI gate. Found nine real bugs on first
  run: invented CodeBuild tag ops, CloudFront UpdatePublicKey at the wrong
  URI, Azure wrong-method/invented routes (BUG-1649..1657) — all fixed.
- **Runtime wire-shape validation (ratcheted)** — `SOCKERLESS_SPEC_VALIDATE`
  arms a capture middleware; per-cloud validators check each response
  member-by-member against the spec output shapes (Smithy / Discovery /
  Swagger with cross-file $ref + allOf + discriminators). CI runs the
  SDK/CLI suites armed and gates the report with
  `scripts/check-spec-violations.sh` against per-cloud allowlists where
  every entry carries a BUG ID. First armed runs filed BUG-1658..1685
  (28 shape-drift bugs: invented members like `Task.networkConfiguration`,
  sockerless wiring leaking onto the wire (`dockerNetworkName`, `simImage`),
  list summaries leaking describe-only fields, wire-name drift like
  `minimumTtl` vs `minimumTTL`) as the open burn-down list.

The runtime layer also exposed a static-gate blind spot: gcp vpcaccess was
never vendored, yet its routes "matched" other docs' trailing-greedy
`v1/{+name}` templates — the misattributed runtime violations led straight
to the missing vendor.

## 2026-06-11 - Simulator conformance + hardening (AWS/GCP/Azure)

A multi-stage effort raising the AWS/GCP/Azure simulators to deep behavioural
fidelity with the real official clients for the implemented slices, then
hardening Go types, the simulator UIs, and CI. Method (see [PLAN.md](PLAN.md)
§ Current Work): read-only audit agents per cloud surface the gaps; each is
verified to reproduce, fixed for real, and covered by a regression test driving
the real SDK/CLI/Terraform client (`simulators/<cloud>/sdk-tests/conformance_roundtrip_test.go`).
Per-fix detail is in [BUGS.md](BUGS.md) (1621-1646) rather than restated here.

- **AWS (Stage 1, merged #537/#538):** round-trip drift (BUG-1621-1629), error
  fidelity (BUG-1630-1635), and pagination via a shared `awsPageExplicit`
  guardrail (BUG-1636).
- **GCP (Stage 2):** round-trip drift (BUG-1637), 409/404/ABORTED error fidelity
  (BUG-1638), pagination + a firestore runQuery operator rewrite (BUG-1639), and
  missing ops — GCS/object PATCH, Spanner Instances.Patch, KMS
  CreateCryptoKeyVersion, CloudFunctions Update/generateUploadUrl, CloudBuild
  ListBuilds, Bigtable modifyColumnFamilies, memorystore/Cloud SQL updateMask
  (BUG-1640).
- **Azure (Stage 3):** round-trip drift incl. ServiceBus ARM server-defaults
  reused from the data-plane (BUG-1641); Tables OData error envelope, EventGrid
  pure-GET, ACR list/listCredentials, blobServices PUT, ListBlobs hierarchy
  (BUG-1642).
- **Go type hardening (Stage 4):** enabled `unconvert` + `wastedassign`
  repo-wide; typed status enums (`ECSTaskStatus`/`ComputeInstanceStatus`/
  `ACIContainerState`); caught + fixed a GCS persistence-helper regression
  (BUG-1643).
- **Simulator UI hardening (Stage 5):** wire-shape drift vs the Go dashboard +
  accurate enum unions across the three sim UIs (BUG-1645).
- **CI (Stage 6):** the simulator **module** unit tests (the AST/guard tests like
  `gcs_internal_test.go`) now run in CI via a `unit-test` Makefile target — the
  gap that let BUG-1643 ship green (BUG-1644). Also fixed an azure terraform
  test that ignored the configured `TERRAFORM_TEST_TIMEOUT` (destroy killed near
  a hardcoded 300s deadline) and a bleephub gh-CLI GraphQL drift (newer
  `gh issue view` sub-issue fields, BUG-1646).

No fakes: false positives were reverted (e.g. a DNS record-set `provisioningState`
the SDK model has no field for) and intractable cases deferred with a documented
reason (GCP cloudbuild/dataflow server-assigned-id 409; the synthetic compute
operation store).

## 2026-06-10 - Bleephub UI GitHub-style restyle + type hardening

- Forked a bleephub-only GitHub-familiar shell (`components/Shell.tsx`):
  top header bar (brand mark + primary nav + theme toggle + sign-out) and a
  per-repo tab row (Code / Issues / Pull requests with counts). The other
  simulator UIs keep the shared editorial-brutalist `AppShell` untouched.
- New GitHub-adjacent token palette in `ui/packages/bleephub/src/index.css`
  (neutral canvas, system sans body / mono for code, distinct teal brand
  accent — not GitHub blue, semantic open=green / merged=purple / closed=red
  state colours). Light is the default (as on github.com); the in-app toggle
  adds `.dark` (Primer-dark-adjacent). `useTheme` gained an additive
  `defaultTheme` arg so bleephub defaults light without affecting other UIs.
- Bleephub-local primitives (`components/ui.tsx`, `components/octicons.tsx`):
  Button / PageTitle / Box / Blankslate / StateLabel / Counter / StatCard /
  Tabs / Modal + original (non-verbatim) SVG glyphs. Every page restyled to
  GitHub conventions; the shared `DataTable` is kept for dense operator tables.
- Type hardening: workflow/job state is now compile-checked Go enums
  (`WorkflowStatus` / `JobStatus` / shared `Result` with named consts in
  `workflows.go`) — wire bytes byte-identical, ~44 literal sites → consts,
  boundary `string()` conversions only where required. UI primitives use
  typed unions (IssuePRState, RepoTab, generic Tabs key).
- Fixed BUG-1619 (three latent CSS custom-property typos that rendered no
  value) and BUG-1620 (AppsPage empty-state strings citing removed
  `/api/v3/bleephub/*` paths). Light + dark screenshots captured via the
  Playwright e2e (now 20 specs incl. a dark-theme pass).

## 2026-06-10 - Bleephub shape-only endpoints filled in

- GPG keys: full CRUD — `POST/GET/DELETE /user/gpg_keys` and `GET
  /users/{username}/gpg_keys` now backed by `MiscStore.gpgKeys` and
  `gpgKeysByUser` maps. Key creation parses `armored_public_key` and
  populates emails from the authenticated user. Ownership enforcement on
  delete. Write-through to `gpg_keys` persistence bucket.
- Pages builds: `POST` trigger creates a real `PagesBuild` record in
  `MiscStore.pagesBuilds`; `GET /builds` lists, `/builds/latest` returns
  newest, `/builds/{build_id}` fetches by ID. Builds are persisted in the
  `pages_builds` bucket keyed by repo full name.
- Audit log: `recordAuditEvent` method appends `AuditEntry` structs to
  `MiscStore.auditLog` with `@timestamp`, `action`, `actor`, `org`,
  `data`, `version` fields. Wired into 16 mutation handlers (repo, org,
  team, hook, secret, issue, PR, release, label, milestone, deployment,
  check_run, GPG key, user key, pages build). Audit endpoint supports
  `phrase` and `actor_id` query filtering. Persisted in `audit_log` bucket.
- Marketplace: plans seeded into `MiscStore.marketplacePlans` on startup via
  `seedDefaultMarketplacePlans`; account endpoint reads from
  `marketplacePurchases` store. Persisted in dedicated buckets.
- OIDC claim keys: added missing `oidcClaimKeys` field to `MiscStore`,
  loaded from `misc` persistence bucket on restart.
- MiscStore: added `persist *Persistence` field, wired in `SetPersistence`.
  New persistence loaders for `misc`, `gpg_keys`, `pages_builds`,
  `audit_log`, `marketplace_plans`, and `marketplace_purchases` buckets.
- 7 new tests: GPG key CRUD, GPG key ownership, Pages builds CRUD, audit
  log recording, audit log from repo creation, marketplace plans, and
  marketplace account.

## 2026-06-09 - Bleephub parity and durability branch planned

- Started `bleephub-parity-storage` as the single planned branch for Bleephub UI/API parity, real Actions cache/artifact behavior, SQLite + PostgreSQL persistence, git storage hardening, S3/MinIO-shaped git content storage, UI auth, and full operator docs.
- Recorded the current audit findings in [STATUS.md](STATUS.md): cache no-ops, catch-all `200 OK`, SQLite-only partial persistence, ignored git storage errors, missing object-store git backend, weak git auth, hard-coded UI admin token, and shape-only long-tail endpoints.
- Reworked [PLAN.md](PLAN.md) and [DO_NEXT.md](DO_NEXT.md) around a multi-session handoff protocol: one PR, one natural commit per subtask, continuity docs updated before and after each completed chunk.

## 2026-06-09 - Bleephub cache and unknown-route behavior

- Replaced the Actions cache no-op handlers with real reserve/upload/finalize,
  lookup, restore-key prefix matching, and download behavior.
- Unknown GitHub API paths stopped returning successful/plain responses and now
  return GitHub-shaped 404 JSON. Non-API unmatched paths return normal HTTP 404.
- The continuity docs now also carry the external-identity rule: observable API
  endpoints, parameters, response fields, UI identity, runner variables, and
  `GITHUB_*` variables must match GitHub/GHES rather than Bleephub-branded
  substitutes, except for internal or operator-only surfaces.

## 2026-06-09 - Bleephub broadened durable state

- [bleephub/persistence.go](bleephub/persistence.go) now writes through all
  public API objects: hooks, hook_deliveries, app_hook_deliveries,
  check_runs, check_suites, check_suite_prefs, repo_secrets,
  workflow_files, pr_reviews, releases, deployments, deployment_statuses,
  environments, pr_review_comments, reactions, projects_v2,
  project_v2_items, and project_v2_fields.
- Sub-stores (ReactionStore, ReleaseStore, DeploymentStore,
  PRReviewCommentStore, ProjectV2Store) now accept `*Persistence` and
  write through on every mutation.
- All new buckets load correctly from disk on restart with proper ID
  counter recovery.

## 2026-06-09 - Bleephub PostgreSQL persistence support

- [bleephub/persistence.go](bleephub/persistence.go) now supports PostgreSQL
  via pgx v5 in addition to SQLite. `BLEEPHUB_DATABASE_URL` activates the
  PostgreSQL backend; `BLEEPHUB_PERSIST=true` continues to activate SQLite.
- A `dbDialect` struct encapsulates dialect-specific SQL (placeholders, DDL,
  type names) so both backends share the same `Persistence` method bodies.
- The PostgreSQL persistence test requires a real PostgreSQL instance
  (`BLEEPHUB_TEST_POSTGRES_URL`) and skips when unavailable. SQLite tests
  continue to pass unchanged.

## 2026-06-09 - Bleephub Actions artifacts REST parity

- Runner-created Actions artifacts now keep repository, GitHub run ID, and
  workflow backend ID metadata, so artifacts can be joined back to real workflow
  runs instead of floating in a global store.
- GitHub REST artifact endpoints now return real stored artifacts for
  repository and run-scoped lists, including pagination, `name` filtering,
  metadata get, delete, `/zip` download redirect, digest, and workflow-run
  fields.
- The separate empty environment-approvals endpoint was recorded in
  [BUGS.md](BUGS.md) as an open fidelity gap rather than being treated as a real
  no-approval signal.

## 2026-06-09 - Bigtable Terraform + AWS execution semantics

- **BUG-1585** was fixed as a coverage gap and provider-routing gap. The GCP Terraform apply stack now declared `google_bigtable_instance` and `google_bigtable_table`, the simulator exposed Bigtable Admin on the official gRPC emulator path used by the Google provider, and the apply harness asserted the provider-returned instance/table IDs. The coverage matrix and `gcp-bigtable` surface table now marked Terraform as direct coverage.
- **BUG-1589** removed synthetic terminal success from four AWS service slices:
  - Batch `SubmitJob` created a real workload container from the registered job definition, tracked the handle, and updated `DescribeJobs` from the actual container exit code.
  - CodeBuild `StartBuild` parsed the official buildspec field for `NO_SOURCE` projects, ran the build commands through a real shell process, and updated `BatchGetBuilds` from process exit status.
  - Glue `StartJobRun` loaded Python shell scripts through the simulator's S3 object store and ran them as real Python processes with the requested job arguments.
  - Step Functions `StartExecution` ran a small ASL interpreter for `Pass`, `Succeed`, `Fail`, and `Wait`; `StopExecution` aborted a real running Wait execution.
- SDK and CLI tests stopped asserting immediate success. They waited on the official service read APIs until terminal state, using real executable inputs: a locally built container image for Batch, inline buildspecs for CodeBuild, S3-backed Python scripts for Glue, and Pass/Fail/Wait ASL definitions for Step Functions.

## 2026-06-09 - Azure/GCP service slices + AWS coverage audit

- Azure added Logic Apps workflow lifecycle/enable/disable/validate plus trigger history, and ACI container group lifecycle/logs/exec backed by real local containers in Docker-runtime mode.
- GCP added Spanner, Dataflow, and Bigtable Admin slices with official SDK and CLI coverage, plus Terraform coverage where provider resources existed.
- AWS residual registered-operation tests were backfilled for SSM, Glue, CodeBuild, Step Functions, CloudWatch Logs, SQS, and ElastiCache operations that had previously lacked public-client regression coverage.
- CI follow-up fixed provider-facing response-shape gaps in Spanner LRO metadata, Logic Apps definitions, ACI repeated fields, and GCP/Azure helper image construction.

## 2026-06-09 - ECS overrides + CloudTrail REST sweep

- ECS `RunTask.overrides.containerOverrides` began following the official ECS SDK/API shape. The simulator echoed task overrides and applied named-container command/environment overrides to the real runtime container for that task.
- CloudTrail recording expanded from central awsJson/query dispatch to path-based REST/RPC service slices, including Lambda, S3, API Gateway, Batch, EFS, CloudFront, Amplify, Route53, and CloudWatch metrics RPCv2. Failed API calls recorded cloud-shaped `errorCode` and `errorMessage`.

## 2026-06-09 - ECS workspace blockers + Entra duplicate UPN

- Azure Entra rejected duplicate `userPrincipalName` values case-insensitively and ROPC used the same resolver.
- AWS ECS Fargate sandbox kept `SYS_CHROOT`, matching real Fargate behavior for sshd-style containers.
- A real CLI regression covered Fargate awsvpc plus ECS managed EBS same-VPC reachability.

## Foundations

Sockerless now includes:

- Docker API-compatible backends for local Docker passthrough and cloud-backed container/FaaS targets.
- High-fidelity AWS, GCP, and Azure local simulators, one binary per cloud, with official SDK/CLI/Terraform coverage tracked in [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).
- A real-execution substrate in `simulators/realexec`: network namespaces, bridges/veth/TAP NICs, IPAM, nftables, Firecracker VM lifecycle, health probes, and load-balancer proxying.
- Cross-cloud OCI `/v2/` registry data-plane implementations for ECR, Artifact Registry, and ACR.
- Bleephub, a GitHub Enterprise-style API simulator covering repos, issues, PRs, Actions, runners, apps, OAuth/OIDC, webhooks, packages, and admin org provisioning.
- Local HTTPS gateway infrastructure through Caddy for providers that require HTTPS endpoint discovery.

Older detailed entries were intentionally compressed out of this file. Use PR descriptions and `git log --oneline --decorate --all` for older implementation history.
