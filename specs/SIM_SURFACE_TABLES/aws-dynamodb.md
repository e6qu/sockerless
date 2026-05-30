# Sim surface — aws-dynamodb

Surface registered in `simulators/aws/dynamodb.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `Action DynamoDB_20120810.CreateTable` | ✓ `simulators/aws/dynamodb.go:169::handleDDBCreateTable` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DynamoDB_20120810.DescribeTable` | ✓ `simulators/aws/dynamodb.go:170::handleDDBDescribeTable` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DynamoDB_20120810.DeleteTable` | ✓ `simulators/aws/dynamodb.go:171::handleDDBDeleteTable` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DynamoDB_20120810.ListTables` | ✓ `simulators/aws/dynamodb.go:172::handleDDBListTables` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DynamoDB_20120810.PutItem` | ✓ `simulators/aws/dynamodb.go:173::handleDDBPutItem` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DynamoDB_20120810.GetItem` | ✓ `simulators/aws/dynamodb.go:174::handleDDBGetItem` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DynamoDB_20120810.UpdateItem` | ✓ `simulators/aws/dynamodb.go:175::handleDDBUpdateItem` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DynamoDB_20120810.DeleteItem` | ✓ `simulators/aws/dynamodb.go:176::handleDDBDeleteItem` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DynamoDB_20120810.Query` | ✓ `simulators/aws/dynamodb.go:177::handleDDBQuery` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DynamoDB_20120810.Scan` | ✓ `simulators/aws/dynamodb.go:178::handleDDBScan` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DynamoDB_20120810.DescribeContinuousBackups` | ✓ `simulators/aws/dynamodb.go:179::handleDDBDescribeContinuousBackups` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DynamoDB_20120810.UpdateContinuousBackups` | ✓ `simulators/aws/dynamodb.go:180::handleDDBUpdateContinuousBackups` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DynamoDB_20120810.DescribeTimeToLive` | ✓ `simulators/aws/dynamodb.go:181::handleDDBDescribeTimeToLive` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DynamoDB_20120810.UpdateTimeToLive` | ✓ `simulators/aws/dynamodb.go:182::handleDDBUpdateTimeToLive` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DynamoDB_20120810.ListTagsOfResource` | ✓ `simulators/aws/dynamodb.go:183::handleDDBListTagsOfResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DynamoDB_20120810.TagResource` | ✓ `simulators/aws/dynamodb.go:184::handleDDBTagResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DynamoDB_20120810.UntagResource` | ✓ `simulators/aws/dynamodb.go:185::handleDDBUntagResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
