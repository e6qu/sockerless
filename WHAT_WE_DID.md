# Sockerless - What We Built

Roadmap [PLAN.md](PLAN.md) - status [STATUS.md](STATUS.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

Detailed historical narrative lives in PR descriptions and `git log`. This file kept the recent chain and a compact foundation summary.

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
