# Simulator conformance + hardening — continuity doc

**Branch:** `feat/sim-conformance-hardening` (one PR, multiple staged commits; CI run per stage).
**Goal:** deep behavioural conformance of the AWS/GCP/Azure simulators against the *real* clients for the implemented slices, plus type + UI hardening. Find bugs → fix → regression-test → document.

This is the **resume artifact** for multi-session work. At the start of every session: read this doc + `BUGS.md` + the last 2 commits, then continue from the current stage's "Next" line.

## Conformance methodology (per surface)

Op-presence is already covered (95 surface tables in `specs/SIM_SURFACE_TABLES/`, SDK+CLI+Terraform per the coverage matrix, CI-enforced). This effort goes **deeper** — the recurring bug class (dropped fields, wrong list envelopes, missing top-level state, non-idempotent reads):

1. **Round-trip drift** — set every writable field via the real SDK/CLI, read it back, assert identical (catches dropped/renamed/defaulted fields).
2. **Idempotency** — `terraform plan -detailed-exitcode` after apply must be 0 (no perpetual diff).
3. **Error fidelity** — NotFound / Conflict / Validation paths return the real wire error code + exception shape the SDK classifies on.
4. **List/pagination fidelity** — envelope keys, `nextToken`/`pageToken`, ordering (newest-first where real cloud does), stable across calls.

Every gap → a `BUGS.md` entry (filed before fix) → real fix → a regression test driving the real client → surface-table/coverage-matrix note if the op status changes.

## Reference adaptors (the conformance oracle)

CI cannot reach real clouds (live-cloud is a separate gated track — BUG-1075). The real official clients encode real-cloud behaviour and ARE reachable in CI:
- AWS: `aws-sdk-go-v2`, `aws` CLI (latest botocore), terraform-provider-aws.
- GCP: cloud client libs, `gcloud`, terraform-provider-google. (Compute/Network apply is Linux-CI-only.)
- Azure: `az*` SDK, `az` CLI, terraform-provider-azurerm. (TF stack is Docker-only; in-Docker `go test -timeout` hardcoded 300s.)

When in doubt about a wire shape, verify with `--debug` / serializer source (`go list -m -f '{{.Dir}}'` then grep serializers.go/deserializers.go), not assumption.

## Stage plan

| Stage | Scope | Status |
|---|---|---|
| 1 | AWS conformance sweep + fixes + regression tests | **DONE** — Batch 1 (PR #537, merged); Batches 2-4 (this branch) |
| 2 | GCP conformance sweep + fixes + regression tests | **DONE** — G1-G4 (BUG-1637/1638/1639/1640); on PR #538 (merged) + this branch (G4) |
| 3 | Azure conformance sweep + fixes + regression tests | **DONE** — A1 (BUG-1641) + A2 (BUG-1642) |
| 4 | Go type hardening across all sims (`docs/GOLANG_STRONG_TYPING.md`) | pending |
| 5 | Simulator UI hardening (aws/azure/gcp UIs) | pending |
| 6 | Wrap: coverage matrix + surface tables + continuity reconcile | pending |

Each stage ends with: `go test ./...` (affected modules) green, golangci-lint v2.10.1 clean, a commit, a push (CI run), and this doc updated.

## Progress log

### Stage 1 — AWS conformance
- **Started:** 2026-06-10.
- **Approach:** walk the 33 AWS surfaces; per surface run the round-trip / error / pagination probes above against the SDK + CLI; file+fix+regress each gap.
- **Findings (from 4 read-only audit agents, to be verified by probe before each fix):**
  Round-trip drift: lambda GetFunction emits `ImageConfig` but SDK reads `ImageConfigResponse`; autoscaling DescribeAutoScalingGroups drops `AutoScalingGroupARN` + `HealthCheckType`; batch DescribeJobs omits `jobArn`; apigateway CreateRestApi/GetRestApi omit `rootResourceId` (+ endpointConfiguration default) → breaks `aws_api_gateway_resource.parent_id`; apigatewayv2 CreateApi omits `apiEndpoint`; apigateway CreateStage drops variables/description; DynamoDB UpdateItem ignores `ReturnValues` (always returns full Attributes); EventBridge PutTargets drops EcsParameters/InputTransformer/RetryPolicy/DeadLetterConfig (`Extra` is `json:"-"`); Kinesis EncryptionType `""` vs `NONE`; RDS restore hardcodes Port 5432; RDS describe omits StorageType/MultiAZ/etc.; ElastiCache single-node redis omits `CacheNodes[].Endpoint`; acm RequestCertificate leaves Options nil (CT-logging default ENABLED); SQS Send/Receive drop DelaySeconds+MessageAttributes; CodeBuild build omits currentPhase/buildNumber; Glue GetJob command-casing; SNS GetTopicAttributes default Policy.
  Error fidelity: batch errors untyped (no ClientException); elbv2 Describe* with explicit-missing ARN returns empty not NotFound exception; ecs DescribeServices ghost cluster → should be ClusterNotFoundException; iam CreatePolicy/CreateRole duplicate name → EntityAlreadyExists; cloudmap NotFound HTTP 404 should be 400; wafv2 duplicate name → WAFDuplicateItemException.
  Pagination: autoscaling/app-autoscaling/batch/eventbridge/iam(ListPolicies,etc.)/cloudtrail/efs list ops ignore NextToken/MaxResults/Marker.
  SFN: no GetExecutionHistory op.
- **Batch 1 — DONE (commit, regression tests green):** BUG-1621 autoscaling ARN+HealthCheckType, BUG-1622 batch jobArn, BUG-1623 apigateway rootResourceId, BUG-1624 apigatewayv2 apiEndpoint, BUG-1625 acm Options CT-logging default, BUG-1626 Kinesis EncryptionType NONE. All 6 reproduced (fail-before/pass-after); regression tests in `simulators/aws/sdk-tests/conformance_roundtrip_test.go`. Build + gofmt clean.
- **Next (Batch 2, round-trip — higher complexity):** lambda GetFunction `ImageConfigResponse` wrapper (response field differs from CreateFunction input `ImageConfig`; nested `{ImageConfig, Error}`); DynamoDB UpdateItem honor `ReturnValues` (currently always returns full Attributes; default NONE → empty); EventBridge PutTargets/ListTargetsByRule carry structured params (EcsParameters/InputTransformer/RetryPolicy/DeadLetterConfig — `EBTarget.Extra` is `json:"-"`, decode+store+emit). Then Batch 3 error fidelity (elbv2/ecs/iam/batch/cloudmap), Batch 4 pagination (autoscaling/batch/eventbridge/iam/efs/cloudtrail/app-autoscaling lists).
- **Batch 2 — DONE:** BUG-1627 lambda ImageConfigResponse, BUG-1628 dynamodb UpdateItem ReturnValues, BUG-1629 eventbridge structured target params.
- **Batch 3 — DONE (error fidelity):** BUG-1630 elbv2 explicit-id NotFound, BUG-1631 ecs ClusterNotFoundException, BUG-1632 iam EntityAlreadyExists, BUG-1633 batch typed ClientException, BUG-1634 cloudmap 404→400, BUG-1635 wafv2 WAFDuplicateItemException.
- **Batch 4 — DONE (pagination):** BUG-1636 — `awsPageExplicit` guardrail helper across iam/eventbridge/batch/autoscaling/app-autoscaling/efs/cloudtrail list ops (paginate only on explicit page size).
- **Stage 1 status: COMPLETE for the audited findings.** All regression tests live in `simulators/aws/sdk-tests/conformance_roundtrip_test.go`. Deferred (lower-value, shared-codepath, documented — pick up if consumer-visible): RDS describe missing fields + restore port, ElastiCache CacheNodes endpoint, apigateway CreateStage variables, SQS DelaySeconds/MessageAttributes, CodeBuild currentPhase/buildNumber, Glue command casing, SNS default Policy, SFN GetExecutionHistory; plus dedicated tests for EFS DescribeMountTargets / autoscaling DescribeScalingActivities / app-AS DescribeScalingPolicies / batch ListJobs+DescribeJobDefinitions+DescribeJobQueues (fixed, covered structurally via shared codepath).
- **Next:** Stage 2 — GCP conformance sweep (same methodology). Run the 4-agent read-only audit over GCP's 23 surfaces, then verify+fix+regress in batches.

### Stage 2 — GCP conformance
- **Started:** 2026-06-10 (on the umbrella PR #538). Same deep-fidelity methodology as Stage 1.
- **Approach:** 4 read-only audit agents over GCP's 23 surfaces (compute/network, data/storage, messaging/eventarc, identity/run/functions) → ranked concrete gaps → verify each with a fail-before probe → fix → pass-after → regression test. GCP terraform compute/network apply is Linux-CI-only; verify via SDK/CLI + `terraform validate` locally, trust CI `tf (gcp)` for apply.
- **Findings (4 read-only audit agents; verify each by probe before fix):**
  Round-trip drift: DNS managedZone drops labels/dnssecConfig/forwarding/peering; BigQuery table drops timePartitioning/clustering/rangePartitioning/view/expirationTime/requirePartitionFilter; Pub/Sub topic drops schemaSettings; Eventarc trigger drops channel; Artifact Registry repo drops labels/cleanupPolicies/dockerConfig/kmsKeyName; Secret Manager Secret drops ttl/expireTime/rotation/topics/annotations/versionAliases (+ UpdateSecret 400s on those masks); Logging metric drops valueExtractor/bucketOptions, sink drops bigqueryOptions; IAM Binding drops `condition`; APIGW IAM etag constant "ACAB".
  Error fidelity: compute insert dup → no 409 ALREADY_EXISTS, delete-missing → no 404, operation GET of bogus name → DONE not 404; pubsub/eventarc/cloudbuild/dataflow duplicate create → no 409; logging sink/metric DELETE-missing → 200 not 404; IAM setIamPolicy ignores etag (no 409 ABORTED).
  Pagination: compute lists read only `pageSize` not `maxResults` (the canonical compute param — existing test green-but-blind); compute-LB lists + DNS rrsets + eventarc + dataflow + logging entries:list + apigateway lists emit no nextPageToken; firestore list/runQuery ignore pageSize/limit/orderBy and only do EQ filters; bigquery/spanner/bigtable/memorystore lists no nextPageToken.
  Missing ops: GCS bucket PATCH + object PATCH; Spanner Instances.Patch 404s; KMS CreateCryptoKeyVersion (+:enable/:disable/:restore); CloudFunctions v2 UpdateFunction(PATCH) + :generateUploadUrl; CloudBuild ListBuilds; Bigtable :modifyColumnFamilies + instance/cluster update; memorystore/SQL patch ignores updateMask / wholesale settings replace.
- **GCP harness:** `simulators/gcp/sdk-tests` (build sim + start; some tests use Docker). Non-compute surfaces run via SDK without real-exec; compute *insert* (networks/instances) needs the Linux real-exec host — use metadata-only compute resources (healthCheck/backendService) or `tf (gcp)` CI for those.
- **Batch G1 — DONE (BUG-1637):** round-trip drift — DNS managedZone, BQ table partitioning/clustering, Pub/Sub schemaSettings, Eventarc channel, AR repo, Secret Manager fields (+UpdateSecret masks), IAM Binding condition, Logging metric fields. 8 SDK regression tests. (Pub/Sub labels/retention were already fine — partial FP.)
- **Batch G2 — DONE (BUG-1638):** error fidelity — compute insert→409 + delete-missing→404 (network/subnet/firewall/address/router/instance + 5 LB inserts), pubsub/eventarc duplicate→409, logging sink/metric delete-missing→404, IAM setIamPolicy stale-etag→409 ABORTED. Shared computeConflict/computeNotFound/gcpIAMETagConflict helpers. Deferred (documented, not faked): compute synthetic/stateless operation store (can't 404 a bogus op name without fabricating a store); cloudbuild/dataflow create use server-assigned ids (name-collision 409 is a different contract).
- **Batch G3 — DONE (BUG-1639):** pagination — compute lists now read `maxResults` (`paginateListCompute`), compute-LB + DNS rrsets + eventarc + dataflow + logging entries:list + bigquery lists emit nextPageToken; firestore list paginates and runQuery was rewritten to honor limit/offset/orderBy + the full operator set (was EQ-only). Guardrail: paginate only on explicit positive page size. 7 SDK regression tests. Deferred (small fixed collections): spanner/bigtable/memorystore/apigateway list tokens.
- **Next — Batch G4 (missing ops, higher complexity):** GCS bucket PATCH + object metadata PATCH; Spanner Instances.Patch (currently 404s); KMS CreateCryptoKeyVersion (+:enable/:disable/:restore, primary-version update); CloudFunctions v2 UpdateFunction (PATCH) + :generateUploadUrl; CloudBuild ListBuilds; Bigtable :modifyColumnFamilies + instance/cluster partialUpdate; memorystore/Cloud SQL patch updateMask merge (currently wholesale replace). These add new routes/ops — verify each adds the real Google method, fail-before/pass-after. Then update this doc AFTER the stage and move to Stage 3 (Azure).
- **Batch G4 — DONE (BUG-1640):** missing ops — GCS bucket+object PATCH, Spanner Instances.Patch, KMS CreateCryptoKeyVersion+UpdateCryptoKeyVersion+:restore, CloudFunctions v2 UpdateFunction+:generateUploadUrl, CloudBuild ListBuilds, Bigtable :modifyColumnFamilies+instance/cluster update, memorystore/Cloud SQL updateMask merge. 11 SDK regression tests. **Stage 2 COMPLETE.**
- **Checkpoint:** Stage 1 + Stage 2 G1-G3 merged in PR #538 (CI green). Stage 2 G4 + Stages 3-6 on branch `feat/sim-conformance-stage2-6` (new umbrella PR).
- **Resume note:** GCP sdk-tests run via `cd simulators/gcp/sdk-tests && GOWORK=off CGO_ENABLED=0 go test -tags noui -run '<pat>' -timeout 15m .` (TestMain builds+starts the sim). compute network/instance INSERT needs the Linux real-exec host (skips on mac via requireNetworkHost) → use metadata-only compute resources (healthCheck/backendService/firewall) for probes. Watch the `simulators-dupl` 200-token gate — factor shared helpers.

### Stage 3 — Azure conformance
- **Started:** 2026-06-11 on branch `feat/sim-conformance-stage2-6`. Same deep-fidelity methodology. ARM REST conventions: `{error:{code,message}}` envelope, `{value,nextLink}` lists, provisioningState/async-op polling. Azure sdk-tests build 3 Docker images in TestMain + use real SDK clients (azcontainerregistry/azsecrets/azkeys) or raw armReq; compute/network create gated behind `azureRequireNetworkHost` (Linux real-exec) → metadata read-back assertions still run; trust `sim (azure)` + `tf (azure)` CI for compute.
- **Findings (4 read-only audit agents; verify each by probe before fix):**
  Round-trip drift: ServiceBus ARM queue/topic/subscription create drops server defaults (lockDuration/defaultMessageTimeToLive/maxDeliveryCount/requiresDuplicateDetection/enableBatchedOperations/autoDeleteOnIdle…) — the data-plane `sb*FromAdminDescription` ALREADY models these, just not wired into ARM create; Storage account drops accessTier/encryption/networkAcls/allowBlobPublicAccess/allowSharedKeyAccess/minimumTlsVersion/publicNetworkAccess/isHnsEnabled (narrow StorageAccountProperties); ACR registry hardcodes publicNetworkAccess=Enabled + zoneRedundancy=Disabled, drops policies/encryption/anonymousPull, sku.tier empty; VM osProfile.adminPassword echoed on GET (write-only, must strip) + ACI secureValue env echoed; private DNS record sets missing provisioningState + auto-SOA empty body; Cosmos no default consistencyPolicy; Redis hardcodes redisVersion 7.0 (Basic/Standard default is 6.0); EventHub hub hardcodes messageRetentionInDays 1 (default 7); EventGrid topic no publicNetworkAccess default + no inputSchema enum validation; Functions site GET surfaces siteConfig.appSettings inline (real strips).
  Error fidelity: Tables data plane uses ARM `{error}` envelope not OData `{odata.error}` + wrong codes (should be TableNotFound/EntityNotFound) → real aztables client can't parse; EventGrid GET topic MUTATES persisted state (side-effecting GET keyed on r.Host); subnet PUT doesn't 404 on missing parent VNet; EventHub partitionCount-decrease should 400.
  Pagination/list: ServiceBus topics/subscriptions/namespaces + EventHub/EventGrid/LogicApps + storage ARM lists + RG list + Entra memberOf emit `{value}` with no nextLink/$top (only SB queues + ACA apps paginate); ListBlobs ignores prefix/delimiter (no BlobPrefix hierarchy) + list entries omit Properties/Metadata.
  Missing ops: ACR registries LIST (by-RG + by-sub) + listCredentials/regenerateCredential; Storage blobServices PUT (blob_properties versioning/changeFeed/deleteRetention/cors); Entra servicePrincipals/applications + collection lists (partly upstream-blocked, BUG-1345); Microsoft.Compute/disks advertised but no handler (no consumer — skip per no-speculative). Monitor alerting absent (no consumer — coverage note only).
- **Batch A1 (round-trip drift):** ServiceBus ARM defaults, Storage account props, ACR registry props+sku.tier, VM/ACI write-only strip, private DNS provisioningState+SOA, Cosmos consistencyPolicy, Redis version, EventHub retention, EventGrid publicNetworkAccess+inputSchema. Then A2 (missing ops: ACR list+credentials, blobServices PUT), A3 (error fidelity + pagination: Tables OData, ListBlobs prefix/delimiter, EventGrid side-effect GET, list nextLink).
- **Batch A1 — DONE (BUG-1641):** round-trip drift (ServiceBus ARM defaults, storage account props, ACR, VM/ACI write-only strip, DNS SOA, Cosmos, Redis, EventHub, EventGrid). 10 tests. FP: DNS record-set provisioningState.
- **Batch A2 — DONE (BUG-1642):** missing ops + error fidelity + pagination (ACR list+listCredentials, blobServices PUT, Tables OData errors, EventGrid pure GET, ServiceBus list pagination, ListBlobs hierarchy). 7 tests. **Stage 3 COMPLETE.** Deferred (small collections): EventHub/EventGrid/LogicApps/storage-ARM/RG list nextLink.

### Stage 4 — Go type hardening — DONE
- **Linters:** added `unconvert` + `wastedassign` to root `.golangci.yml` (verified 0-fallout across all CI-linted modules: 3 sims + shared + 7 backends + bleephub + agent + cmd; fixed ~17 trivial hits). Rejected `usestdlibvars` (65 hits — too much churn) and `nilerr` (10 intentional io.Writer/Walk patterns) with reasons.
- **Typed enums** (one contained, wire-identical field per sim; mistyped state literal → compile error): AWS `ECSTaskStatus` (ecs.go LastStatus/DesiredStatus, ~23 sites), GCP `ComputeInstanceStatus` (compute.go Status, 6), Azure `ACIContainerState` (containerinstance.go State, 11). JSON bytes byte-identical.
- **Caught + fixed BUG-1643** (a Batch-G4 GCS regression on main: metadata PATCH bypassed `persistGCSObject`) + filed **BUG-1644** (CI doesn't run sim-module unit tests — the gap that let 1643 ship green; fix in Stage 6).
- All 3 sim modules: build + module unit tests + golangci-lint (new config) + dupl all green.

### Stage 5 — Simulator UI hardening — DONE
- Cross-checked all 3 sim UIs' `api.ts` against the Go dashboard wire shapes; fixed BUG-1645 (gcp `severity` should be optional — server omits for DEFAULT; azure `MonitorLogRow` values are `string` not `unknown`). Narrowed stringly enums to unions matching the values the server actually emits (aws ECSTask.status to the real 5, LambdaFunction.state; gcp CloudFunction.state, LogEntry.severity LogSeverity) — accuracy over breadth (a too-wide union is worse than `string`). typecheck + build green for all 3.

### Stage 6 — Wrap — DONE
- **CI gap fixed (BUG-1644):** added a `unit-test` target to each sim Makefile (`go test -tags noui ./...` on the module — fast, no Docker/real-exec) and wired a "Run module unit tests" step into the `sim (gcp/azure)` + `sim (aws sdk)` CI jobs, so package-internal guard/unit tests (e.g. `gcs_internal_test.go`) now run in CI. This is the gap that let BUG-1643 ship green.
- **Coverage matrix:** `scripts/check-simulator-coverage-matrix.sh` verified green. Surface tables (`specs/SIM_SURFACE_TABLES/`) intentionally NOT bulk-regenerated — the seed script over-generates per-sub-file tables (spurious `azure-arm_lro` etc.) and churns ~1000 lines; per their own policy (✗ rows added when an audit/issue surfaces them) leaving them as-is is fine and keeps the gate green.
- **All stages complete.**

## Summary (all stages)
| Stage | Bugs | Status |
|---|---|---|
| 1 AWS conformance | 1621-1636 (round-trip, error, pagination) | done (#537/#538) |
| 2 GCP conformance | 1637-1640 (round-trip, error, pagination, missing-ops) | done (#538 + here) |
| 3 Azure conformance | 1641-1642 (round-trip, error/ops/pagination) | done |
| 4 Go type hardening | linters + 3 typed enums; +1643 (regression) | done |
| 5 Simulator UI hardening | 1645 (wire drift + unions) | done |
| 6 Wrap | 1644 (CI gap) fixed; matrix green | done |

Every fix carries an SDK regression test driving the real client (`simulators/{aws,gcp,azure}/sdk-tests/conformance_roundtrip_test.go`). Two deferrals documented-not-faked (GCP synthetic compute op store; cloudbuild/dataflow server-assigned ids); false positives reverted (DNS record-set provisioningState; Pub/Sub scalars).
