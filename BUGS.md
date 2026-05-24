# Known Bugs

**1162 filed · 1160 fixed · 2 open · 2 false positives.**

Standing rule: every CI / live-cloud failure lands here with a one-liner *before* any fix attempt. Workarounds, fakes, placeholders, silent fallbacks, skips, and incomplete implementations are all bugs and get the same treatment. Per-bug fix detail beyond the one-liner: `git log <commit>` or the linked PR.

Live status (cells, branch, milestone) lives in [STATUS.md](STATUS.md). Vibe-pattern numbers reference `docs/VIBE_CODING.md`.

## Open

| ID | Sev | Area | Pattern | One-liner |
|----|-----|------|---------|-----------|
| 1075 | P2 | live-cloud cells red for cloudrun Services + ACA Apps + AZF + Lambda service-mesh + ACA/AZF Azure AD | 6 (untested in real cloud) | Lambda is the only backend with a green live-cloud cell. Cloud Run Services + ACA Apps + AZF cloud-DNS + Lambda service-mesh + ACA/AZF Azure AD remain unvalidated against real clouds. The preflight/runbook work is documented, but the cells must not be marked green without a real run against authenticated cloud projects/subscriptions. |
| 1104 | P0 | Audit-cadence meta tracker — perpetual | meta | Every major phase runs the specialist-skill pass against the diff. This BUG stays Open as the audit-cadence reminder; it closes when there's no meaningful new sim work for ≥ 6 phases (i.e., simulator surface is genuinely complete + matches every active SDK contract). |

## Recently closed (last phase only — older history lives in PR descriptions + `git log`)

Phase 178 (PR #211, 16 commits) closed BUG-1148..1160 — 9 community-filed issues (#196 reopen + #203..#210) + 4 class-of-bug remediations:

- **1148** AWS S3 multipart `ListParts` (#196 reopened, postmortem of 1138 + 1142). New `handleS3ListParts` returns paged-shape XML; new surface table `aws-s3-multipart.md` covers every multipart op.
- **1149** Azure KV `GET /secrets/{name}/versions` wrong shape + PUT overwrites (#203). `kvSecretStored` refactored to versioned chain; paged `SecretListResult`.
- **1150** AWS API Gateway v2 `POST /v2/apis/{id}/deployments` shadowed by S3 multipart dispatcher (#204). Register the missing v2 deployment routes + known-bucket gate on S3's wildcard dispatchers.
- **1151** Azure KV PATCH + soft-delete `/deletedsecrets/` (#205). State machine active → soft-deleted → recovered / purged.
- **1152** Azure App Service `/sites/{name}/config/{appsettings,connectionstrings,web}` (#206).
- **1153** AWS RDS snapshot family + SNS SetTopicAttributes + SQS PurgeQueue (#207). RDS snapshot state machine; SNS per-topic Attributes round-trip; SQS Purge preserves Attributes/Tags.
- **1154** AWS awsQuery dispatch by (Version, Action) (#208). RDS / ElastiCache / SNS migrated to RegisterVersioned with canonical API versions.
- **1155** GCP Cloud SQL backupRuns + clone + selfLink scheme; Memorystore upgrade + failover; Pub/Sub Snapshot CRUD; API Gateway IAM v1 (#209).
- **1156** Azure PG FlexibleServer configurations + APIM operations/backends/namedValues + Cache Redis firewallRules + listKeys (#210).
- **1157** Proactive surface-table seed via `scripts/seed-surface-tables.sh` — 46 tables covering ~700 ops.
- **1158** `mux-overlap-scan` skill + scanner script + pre-commit hook (warn mode).
- **1159** `sim-canonical-config-test` extended with paged-iterator rule + `paged-shape verified` column on surface tables.
- **1160** `sim-state-machine-completeness` skill — every stateful resource type models the documented state machine.
- **1161** CI gap — `simulators/{aws,gcp,azure}/terraform-tests/` exist with 40 + 16 + 21 stock-provider resources, but the CI `sim` job only invokes `make sdk-test` + `make cli-test` per cloud, never `make tf-test`. Three recent reopens (BUG-1098 / 1099 / 1142 / 1147) all surfaced because the terraform-provider's call sequence differs materially from the SDK's. Fifth class-of-bug remediation: add `tf (aws|gcp|azure)` matrix job to `.github/workflows/ci.yml` running `make tf-test` so the tf-provider call shape gets exercised every push.
- **1162** Cloud Run + GCF FaaS smoke harnesses (`backends/cloudrun/integration_test.go`, `backends/cloudrun-functions/integration_test.go`) allocated a free port for the sim's HTTP listener but let the gRPC port default to `HTTP+1`. CI flake on `test (cloudrun faas-smoke)`: simulator-gcp's gRPC bound to :HTTP+1 hit `bind: address already in use` and `log.Fatalf` killed the whole sim process, causing the /health check to time out. Pre-allocate a separate free port via `findFreePort()` and pass via `SIM_GCP_GRPC_PORT` — same pattern the SDK + terraform-tests harnesses already use.

Older closed BUGs: 1098..1147 across Phases 173–177. See `WHAT_WE_DID.md` per-phase narrative + PR descriptions for fix detail.

## False positives

| Area | Finding | Why it's not a bug |
|------|---------|--------------------|
| `backends/aca/azure.go::fakeCredential` | Returns literal `"fake-token"` against simulator endpoints. | Sims don't verify bearer tokens — would require real Azure AD endpoint not emulated. Credential wired only via `newAzureClientsWithEndpoint` (sim path); production uses `azidentity.NewDefaultAzureCredential`. |
| `cmd/sockerless-admin/api_observability.go::envOrDefault` | Returns canonical OTel resource-attribute name when unset. | Documented default-value helper, not an error-hiding fallback. No silent failure mode. |

## Class-of-bug rules

- **Backend ↔ host primitive must match (P0).** ECS in ECS, Lambda in Lambda, Cloud Run in Cloud Run, GCF in CRF, ACA in ACA, AZF in AZF.
- **No fakes / no fallbacks / no skips.** Synthetic exit codes, silent shims, fake-data fallbacks, conditional `t.Skip` for missing config — all file as bugs.
- **Cross-cloud sweep on every find.** When a pattern surfaces in one backend, the same code paths in the other 5 backends / 3 sims get checked in the same commit.
- **HTTP 500 reserved for unexpected panics.** Never return 5xx as a designed failure path.
- **Doc-only fixes are unsafe when the cloud rejects the config.** Verify cloud-acceptable, not just sockerless-controllable.
- **External test fixtures must use the real client.** The `gh` CLI test harness uses real `gh repo create` against bleephub, not URL hackery.
- **Closed enumeration → full-table audit.** Every community-filed issue against a closed operation table (subresources, KV ops, Pub/Sub verbs, etc.) gets the full table reviewed before "fixed" is claimed (`surface-table-completeness` skill).
- **Reopens carry a postmortem trail.** Every reopened community-filed issue is P0 by default; the BUG entry must include (a) what test passed but should have failed, (b) what SDK code path was missed, (c) what new canonical-client test catches the regression (`reopen-postmortem` skill).
- **List* ops use paged-iterator tests.** Single-record envelopes from List endpoints silently pass `.Value[0]`-style tests (`sim-canonical-config-test` paged-iterator rule).
- **Stateful resources model their state machine.** Real cloud resources transition through documented lifecycle states; sim implementations carry the state field + transitions (`sim-state-machine-completeness` skill).
- **Mux pattern overlap is a real-bug class.** The collapsed-port sim has no DNS-level service isolation; root-greedy wildcards can shadow other services. Run `scripts/scan-mux-overlap.sh` (also wired into pre-commit) when adding routes.
