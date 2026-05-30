# Sim surface — aws-cloudwatch

Surface registered in `simulators/aws/cloudwatch.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `Action Logs_20140328.CreateLogGroup` | ✓ `simulators/aws/cloudwatch.go:69::handleCWCreateLogGroup` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `Action Logs_20140328.DescribeLogGroups` | ✓ `simulators/aws/cloudwatch.go:70::handleCWDescribeLogGroups` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `Action Logs_20140328.DeleteLogGroup` | ✓ `simulators/aws/cloudwatch.go:71::handleCWDeleteLogGroup` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `Action Logs_20140328.CreateLogStream` | ✓ `simulators/aws/cloudwatch.go:72::handleCWCreateLogStream` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `Action Logs_20140328.DescribeLogStreams` | ✓ `simulators/aws/cloudwatch.go:73::handleCWDescribeLogStreams` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `Action Logs_20140328.PutLogEvents` | ✓ `simulators/aws/cloudwatch.go:74::handleCWPutLogEvents` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `Action Logs_20140328.GetLogEvents` | ✓ `simulators/aws/cloudwatch.go:75::handleCWGetLogEvents` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `Action Logs_20140328.FilterLogEvents` | ✓ `simulators/aws/cloudwatch.go:76::handleCWFilterLogEvents` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `Action Logs_20140328.PutRetentionPolicy` | ✓ `simulators/aws/cloudwatch.go:77::handleCWPutRetentionPolicy` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `Action Logs_20140328.ListTagsForResource` | ✓ `simulators/aws/cloudwatch.go:78::handleCWListTagsForResource` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `Action Logs_20140328.TagResource` | ✓ `simulators/aws/cloudwatch.go:79::handleCWTagResource` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
