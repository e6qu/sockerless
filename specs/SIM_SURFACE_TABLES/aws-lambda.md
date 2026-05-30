# Sim surface — aws-lambda

Surface registered in `simulators/aws/lambda.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `POST /2015-03-31/functions` | ✓ `simulators/aws/lambda.go:91::handleLambdaCreateFunction` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `GET /2015-03-31/functions/{name}` | ✓ `simulators/aws/lambda.go:92::handleLambdaGetFunction` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `DELETE /2015-03-31/functions/{name}` | ✓ `simulators/aws/lambda.go:93::handleLambdaDeleteFunction` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `PUT /2015-03-31/functions/{name}/configuration` | ✓ `simulators/aws/lambda.go:94::handleLambdaUpdateFunctionConfiguration` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `POST /2015-03-31/functions/{name}/invocations` | ✓ `simulators/aws/lambda.go:95::handleLambdaInvoke` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `GET /2015-03-31/functions` | ✓ `simulators/aws/lambda.go:96::handleLambdaListFunctions` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `GET /2015-03-31/functions/` | ✓ `simulators/aws/lambda.go:97::handleLambdaListFunctions` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `GET /2017-03-31/tags/{arn...}` | ✓ `simulators/aws/lambda.go:98::handleLambdaListTags` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `POST /2017-03-31/tags/{arn...}` | ✓ `simulators/aws/lambda.go:99::handleLambdaTagResource` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `DELETE /2017-03-31/tags/{arn...}` | ✓ `simulators/aws/lambda.go:100::handleLambdaUntagResource` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `POST /2015-03-31/functions/{name}/versions` | ✓ `simulators/aws/lambda.go:103::handleLambdaPublishVersion` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `GET /2015-03-31/functions/{name}/versions` | ✓ `simulators/aws/lambda.go:104::handleLambdaListVersions` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `POST /2015-03-31/functions/{name}/aliases` | ✓ `simulators/aws/lambda.go:105::handleLambdaCreateAlias` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `GET /2015-03-31/functions/{name}/aliases` | ✓ `simulators/aws/lambda.go:106::handleLambdaListAliases` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `GET /2015-03-31/functions/{name}/aliases/{alias}` | ✓ `simulators/aws/lambda.go:107::handleLambdaGetAlias` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `PUT /2015-03-31/functions/{name}/aliases/{alias}` | ✓ `simulators/aws/lambda.go:108::handleLambdaUpdateAlias` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `DELETE /2015-03-31/functions/{name}/aliases/{alias}` | ✓ `simulators/aws/lambda.go:109::handleLambdaDeleteAlias` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `POST /2015-03-31/functions/{name}/policy` | ✓ `simulators/aws/lambda.go:110::handleLambdaAddPermission` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `GET /2015-03-31/functions/{name}/policy` | ✓ `simulators/aws/lambda.go:111::handleLambdaGetPolicy` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `DELETE /2015-03-31/functions/{name}/policy/{statement}` | ✓ `simulators/aws/lambda.go:112::handleLambdaRemovePermission` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `POST /2021-10-31/functions/{name}/url` | ✓ `simulators/aws/lambda.go:113::handleLambdaCreateFunctionUrlConfig` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `GET /2021-10-31/functions/{name}/url` | ✓ `simulators/aws/lambda.go:114::handleLambdaGetFunctionUrlConfig` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `PUT /2021-10-31/functions/{name}/url` | ✓ `simulators/aws/lambda.go:115::handleLambdaUpdateFunctionUrlConfig` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `DELETE /2021-10-31/functions/{name}/url` | ✓ `simulators/aws/lambda.go:116::handleLambdaDeleteFunctionUrlConfig` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
