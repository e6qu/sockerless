# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Current State

- Branch: `feat/aws-sim-real-cloudwatch-metrics` (PR pending — real CloudWatch metrics, BUG-1475).
- Last merged: PR #431 (IAM policy simulation, #427).
- AWS CloudWatch real metrics (BUG-1475, audit follow-up): removed `computeECSMetric` (the fabricated `0.15×CPU`/`0.25×mem`) — `GetMetricData` now serves real `PutMetricData` datapoints for every namespace (ECS included; empty if nothing pushed). Added real CloudWatch period-bucketing + statistic (`cwAggregate`/`cwApplyStat`: Average/Sum/Minimum/Maximum/SampleCount over `[StartTime,EndTime)`). Two pre-existing protocol bugs the missing test hid, now fixed: (a) `PutMetricData` request bodies are gzip-compressed CBOR (`cwReadBody` decompresses); (b) the response must encode timestamps as CBOR **tag-1** not a bare uint (`cwEncMode` with `TimeTag: EncTagRequired`). SDK round-trip test (Average/Sum/Min/Max/SampleCount + ECS de-fabrication). Added the `cloudwatch` SDK module to sdk-tests/go.mod.
- **Possible follow-up (the metrics CLI gap):** the aws CLI (botocore) uses the legacy **query protocol** (`Action=PutMetricData`, XML) for CloudWatch — NOT the rpc-v2-cbor the Go SDK uses — so `aws cloudwatch put-metric-data`/`get-metric-statistics` currently return `InvalidAction`. Implementing the query-protocol metric ops (PutMetricData + GetMetricStatistics XML, backed by the same `cwMetrics` store) would make the CLI work. Separate, sizable.
- After this: no actionable consumer issues (only #394, upstream-blocked). Other audit items (IMDS accountId, GCS preconditions, ACR checkNameAvailability) or await the consumer's next batch.
- Open GitHub issues: #394 (upstream-blocked).
- Open BUG trackers: BUG-1075, BUG-1104, BUG-1345.
- BUG counters: 1475 filed · 1431 fixed · 5 open · 4 false positives.

## Recently Completed

| PR | Description |
|----|-------------|
| #401 | bleephub auth conformance: session/CSRF OAuth flow + site-admin org endpoint |
| #402 | Phase C (AWS): pagination on 12 list endpoints |
| #403 | Phase C (GCP/Azure): pagination on GCP/Azure list endpoints |
| #404 | Phase D: error envelope fidelity + negative-path SDK error classification tests |
| #405 | Phase E+F: Azure KV data-plane CLI tests; 12 bleephub surface table files; webhook schema fixes (BUG-1396–1398) |

## Deferred / Blocked

| Item | Blocker |
|------|---------|
| `azuread_group` / `azuread_user` Terraform tests (BUG-1345) | Upstream: no `microsoft_graph_endpoint` override in `hashicorp/terraform-provider-azuread` (issue #1837 upstream, issue #394 here) |
| Live-cloud validation (BUG-1075) | Requires authenticated real-cloud runs; no timeline |

## What to Work On Next

**Phase G — New cloud service slices** (see PLAN.md for candidates). Each new slice ships with SDK + CLI + Terraform coverage per standard contract. No scope finalised yet — discuss with user before starting.

## Start Checklist (every session)

1. `git fetch origin && git checkout main && git reset --hard origin/main`
2. `gh issue list --state open --limit 30`
3. Check current open BUGs and the counter in `BUGS.md`.
4. Create a fresh branch from `origin/main`.
5. File BUG entries in `BUGS.md` **before** writing any code.
6. Run `go test ./...` in affected modules after every meaningful edit.

## Rules

- No stubs, fakes, mocks, synthetic responses, or silent fallbacks.
- Every new simulator public API path: SDK + CLI + Terraform coverage where those surfaces exist.
- One PR per cloud area; do not split into sub-phases.
- User merges PRs — never run `gh pr merge`.
- Rebase PR branch on `origin/main` before push.
- File bugs before fixes, not after.
