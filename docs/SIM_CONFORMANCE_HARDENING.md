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
| 2 | GCP conformance sweep + fixes + regression tests | pending |
| 3 | Azure conformance sweep + fixes + regression tests | pending |
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
- Not started.

### Stage 3 — Azure conformance
- Not started.

### Stage 4 — Go type hardening
- Not started. Candidates surface during stages 1-3 (stringly-typed states, bare-ID transposition, `map[string]any` request decode). Apply typed enums/IDs/sealed sums per `docs/GOLANG_STRONG_TYPING.md`.

### Stage 5 — Simulator UI hardening
- Not started. 3 UIs (`ui/packages/simulator-{aws,gcp,azure}`, ~8 TS files each on a shared core). Tighten types, fix bugs, verify.

### Stage 6 — Wrap
- Not started. Reconcile `specs/SIM_TEST_COVERAGE_MATRIX.md` + surface tables; final continuity pass.
