# Sim surface — aws-dynamodb

Surface registered in `simulators/aws/dynamodb.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with a BUG / deferred-subtask reference; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no terraform-provider resource for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `Action DynamoDB_20120810.CreateTable` | ✓ `simulators/aws/dynamodb.go:169::handleDDBCreateTable` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DynamoDB_20120810.DescribeTable` | ✓ `simulators/aws/dynamodb.go:170::handleDDBDescribeTable` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DynamoDB_20120810.DeleteTable` | ✓ `simulators/aws/dynamodb.go:171::handleDDBDeleteTable` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DynamoDB_20120810.ListTables` | ✓ `simulators/aws/dynamodb.go:172::handleDDBListTables` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DynamoDB_20120810.PutItem` | ✓ `simulators/aws/dynamodb.go:173::handleDDBPutItem` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DynamoDB_20120810.GetItem` | ✓ `simulators/aws/dynamodb.go:174::handleDDBGetItem` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DynamoDB_20120810.UpdateItem` | ✓ `simulators/aws/dynamodb.go:175::handleDDBUpdateItem` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DynamoDB_20120810.DeleteItem` | ✓ `simulators/aws/dynamodb.go:176::handleDDBDeleteItem` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DynamoDB_20120810.Query` | ✓ `simulators/aws/dynamodb.go:177::handleDDBQuery` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DynamoDB_20120810.Scan` | ✓ `simulators/aws/dynamodb.go:178::handleDDBScan` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DynamoDB_20120810.DescribeContinuousBackups` | ✓ `simulators/aws/dynamodb.go:179::handleDDBDescribeContinuousBackups` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DynamoDB_20120810.UpdateContinuousBackups` | ✓ `simulators/aws/dynamodb.go:180::handleDDBUpdateContinuousBackups` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DynamoDB_20120810.DescribeTimeToLive` | ✓ `simulators/aws/dynamodb.go:181::handleDDBDescribeTimeToLive` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DynamoDB_20120810.UpdateTimeToLive` | ✓ `simulators/aws/dynamodb.go:182::handleDDBUpdateTimeToLive` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DynamoDB_20120810.ListTagsOfResource` | ✓ `simulators/aws/dynamodb.go:183::handleDDBListTagsOfResource` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DynamoDB_20120810.TagResource` | ✓ `simulators/aws/dynamodb.go:184::handleDDBTagResource` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DynamoDB_20120810.UntagResource` | ✓ `simulators/aws/dynamodb.go:185::handleDDBUntagResource` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |

## Open subtasks staged forward

- sdk-test / tf-test columns are ✗-with-deferral for every row above. Each subsequent surface-touching PR fills in the column for the rows it covers; remaining ✗s are tracked under BUG-1159 (paged-iterator sweep) + BUG-1147 (tf-test parity sweep).
- Missing ops (not in HandleFunc but documented by the cloud provider) get ✗ rows added when a community-filed issue surfaces them or a periodic audit lands a sweep.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
