# Sim surface — aws-lambda

Surface registered in `simulators/aws/lambda.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with a BUG / deferred-subtask reference; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no terraform-provider resource for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `POST /2015-03-31/functions` | ✓ `simulators/aws/lambda.go:91::handleLambdaCreateFunction` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /2015-03-31/functions/{name}` | ✓ `simulators/aws/lambda.go:92::handleLambdaGetFunction` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /2015-03-31/functions/{name}` | ✓ `simulators/aws/lambda.go:93::handleLambdaDeleteFunction` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `PUT /2015-03-31/functions/{name}/configuration` | ✓ `simulators/aws/lambda.go:94::handleLambdaUpdateFunctionConfiguration` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /2015-03-31/functions/{name}/invocations` | ✓ `simulators/aws/lambda.go:95::handleLambdaInvoke` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /2015-03-31/functions` | ✓ `simulators/aws/lambda.go:96::handleLambdaListFunctions` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /2015-03-31/functions/` | ✓ `simulators/aws/lambda.go:97::handleLambdaListFunctions` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /2017-03-31/tags/{arn...}` | ✓ `simulators/aws/lambda.go:98::handleLambdaListTags` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /2017-03-31/tags/{arn...}` | ✓ `simulators/aws/lambda.go:99::handleLambdaTagResource` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /2017-03-31/tags/{arn...}` | ✓ `simulators/aws/lambda.go:100::handleLambdaUntagResource` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /2015-03-31/functions/{name}/versions` | ✓ `simulators/aws/lambda.go:103::handleLambdaPublishVersion` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /2015-03-31/functions/{name}/versions` | ✓ `simulators/aws/lambda.go:104::handleLambdaListVersions` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /2015-03-31/functions/{name}/aliases` | ✓ `simulators/aws/lambda.go:105::handleLambdaCreateAlias` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /2015-03-31/functions/{name}/aliases` | ✓ `simulators/aws/lambda.go:106::handleLambdaListAliases` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /2015-03-31/functions/{name}/aliases/{alias}` | ✓ `simulators/aws/lambda.go:107::handleLambdaGetAlias` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `PUT /2015-03-31/functions/{name}/aliases/{alias}` | ✓ `simulators/aws/lambda.go:108::handleLambdaUpdateAlias` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /2015-03-31/functions/{name}/aliases/{alias}` | ✓ `simulators/aws/lambda.go:109::handleLambdaDeleteAlias` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /2015-03-31/functions/{name}/policy` | ✓ `simulators/aws/lambda.go:110::handleLambdaAddPermission` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /2015-03-31/functions/{name}/policy` | ✓ `simulators/aws/lambda.go:111::handleLambdaGetPolicy` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /2015-03-31/functions/{name}/policy/{statement}` | ✓ `simulators/aws/lambda.go:112::handleLambdaRemovePermission` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /2021-10-31/functions/{name}/url` | ✓ `simulators/aws/lambda.go:113::handleLambdaCreateFunctionUrlConfig` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /2021-10-31/functions/{name}/url` | ✓ `simulators/aws/lambda.go:114::handleLambdaGetFunctionUrlConfig` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `PUT /2021-10-31/functions/{name}/url` | ✓ `simulators/aws/lambda.go:115::handleLambdaUpdateFunctionUrlConfig` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /2021-10-31/functions/{name}/url` | ✓ `simulators/aws/lambda.go:116::handleLambdaDeleteFunctionUrlConfig` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |

## Open subtasks staged forward

- sdk-test / tf-test columns are ✗-with-deferral for every row above. Each subsequent surface-touching PR fills in the column for the rows it covers; remaining ✗s are tracked under BUG-1159 (paged-iterator sweep) + BUG-1147 (tf-test parity sweep).
- Missing ops (not in HandleFunc but documented by the cloud provider) get ✗ rows added when a community-filed issue surfaces them or a periodic audit lands a sweep.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
