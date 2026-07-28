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
| `POST /2015-03-31/functions` | ✓ `simulators/aws/lambda.go:359::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /2015-03-31/functions/{name}` | ✓ `simulators/aws/lambda.go:360::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /2015-03-31/functions/{name}` | ✓ `simulators/aws/lambda.go:361::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /2015-03-31/functions/{name}/configuration` | ✓ `simulators/aws/lambda.go:362::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /2015-03-31/functions/{name}/invocations` | ✓ `simulators/aws/lambda.go:363::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /2015-03-31/functions` | ✓ `simulators/aws/lambda.go:364::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /2015-03-31/functions/` | ✓ `simulators/aws/lambda.go:365::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /2017-03-31/tags/{arn...}` | ✓ `simulators/aws/lambda.go:366::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /2017-03-31/tags/{arn...}` | ✓ `simulators/aws/lambda.go:367::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /2017-03-31/tags/{arn...}` | ✓ `simulators/aws/lambda.go:368::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /2015-03-31/functions/{name}/versions` | ✓ `simulators/aws/lambda.go:371::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /2015-03-31/functions/{name}/versions` | ✓ `simulators/aws/lambda.go:372::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /2015-03-31/functions/{name}/aliases` | ✓ `simulators/aws/lambda.go:373::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /2015-03-31/functions/{name}/aliases` | ✓ `simulators/aws/lambda.go:374::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /2015-03-31/functions/{name}/aliases/{alias}` | ✓ `simulators/aws/lambda.go:375::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /2015-03-31/functions/{name}/aliases/{alias}` | ✓ `simulators/aws/lambda.go:376::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /2015-03-31/functions/{name}/aliases/{alias}` | ✓ `simulators/aws/lambda.go:377::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /2015-03-31/functions/{name}/policy` | ✓ `simulators/aws/lambda.go:378::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /2015-03-31/functions/{name}/policy` | ✓ `simulators/aws/lambda.go:379::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /2015-03-31/functions/{name}/policy/{statement}` | ✓ `simulators/aws/lambda.go:380::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /2021-10-31/functions/{name}/url` | ✓ `simulators/aws/lambda.go:381::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /2021-10-31/functions/{name}/url` | ✓ `simulators/aws/lambda.go:382::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /2021-10-31/functions/{name}/url` | ✓ `simulators/aws/lambda.go:383::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /2021-10-31/functions/{name}/url` | ✓ `simulators/aws/lambda.go:384::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /2021-10-31/functions/{name}/urls` | ✓ `simulators/aws/lambda.go:385::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /2015-03-31/event-source-mappings` | ✓ `simulators/aws/lambda.go:388::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /2015-03-31/event-source-mappings` | ✓ `simulators/aws/lambda.go:389::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /2015-03-31/event-source-mappings/{uuid}` | ✓ `simulators/aws/lambda.go:390::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /2015-03-31/event-source-mappings/{uuid}` | ✓ `simulators/aws/lambda.go:391::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /2015-03-31/event-source-mappings/{uuid}` | ✓ `simulators/aws/lambda.go:392::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /2018-10-31/layers/{layer}/versions` | ✓ `simulators/aws/lambda.go:397::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /2018-10-31/layers/{layer}/versions` | ✓ `simulators/aws/lambda.go:398::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /2018-10-31/layers/{layer}/versions/{version}` | ✓ `simulators/aws/lambda.go:399::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /2018-10-31/layers/{layer}/versions/{version}` | ✓ `simulators/aws/lambda.go:400::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /2018-10-31/layers` | ✓ `simulators/aws/lambda.go:405::cloudTrailRecordedRESTDynamic` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /2017-10-31/functions/{name}/concurrency` | ✓ `simulators/aws/lambda.go:408::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /2019-09-30/functions/{name}/concurrency` | ✓ `simulators/aws/lambda.go:409::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /2017-10-31/functions/{name}/concurrency` | ✓ `simulators/aws/lambda.go:410::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /2019-09-25/functions/{name}/event-invoke-config` | ✓ `simulators/aws/lambda_extras2.go:28::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /2019-09-25/functions/{name}/event-invoke-config` | ✓ `simulators/aws/lambda_extras2.go:29::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /2019-09-25/functions/{name}/event-invoke-config` | ✓ `simulators/aws/lambda_extras2.go:30::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /2019-09-25/functions/{name}/event-invoke-config` | ✓ `simulators/aws/lambda_extras2.go:31::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /2019-09-25/functions/{name}/event-invoke-config/list` | ✓ `simulators/aws/lambda_extras2.go:32::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /2019-09-30/functions/{name}/provisioned-concurrency` | ✓ `simulators/aws/lambda_extras2.go:35::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /2019-09-30/functions/{name}/provisioned-concurrency` | ✓ `simulators/aws/lambda_extras2.go:36::cloudTrailRecordedRESTDynamic` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /2019-09-30/functions/{name}/provisioned-concurrency` | ✓ `simulators/aws/lambda_extras2.go:37::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /2020-04-22/code-signing-configs` | ✓ `simulators/aws/lambda_extras2.go:49::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /2020-04-22/code-signing-configs` | ✓ `simulators/aws/lambda_extras2.go:50::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /2020-04-22/code-signing-configs/{arn...}` | ✓ `simulators/aws/lambda_extras2.go:51::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /2020-04-22/code-signing-configs/{arn...}` | ✓ `simulators/aws/lambda_extras2.go:52::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /2020-04-22/code-signing-configs/{arn...}` | ✓ `simulators/aws/lambda_extras2.go:53::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /2020-06-30/functions/{name}/code-signing-config` | ✓ `simulators/aws/lambda_extras2.go:56::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /2020-06-30/functions/{name}/code-signing-config` | ✓ `simulators/aws/lambda_extras2.go:57::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /2020-06-30/functions/{name}/code-signing-config` | ✓ `simulators/aws/lambda_extras2.go:58::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /2021-07-20/functions/{name}/runtime-management-config` | ✓ `simulators/aws/lambda_extras2.go:61::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /2021-07-20/functions/{name}/runtime-management-config` | ✓ `simulators/aws/lambda_extras2.go:62::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /2016-08-19/account-settings` | ✓ `simulators/aws/lambda_extras2.go:65::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /2016-08-19/account-settings/` | ✓ `simulators/aws/lambda_extras2.go:66::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /2024-08-31/functions/{name}/recursion-config` | ✓ `simulators/aws/lambda_extras2.go:69::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /2024-08-31/functions/{name}/recursion-config` | ✓ `simulators/aws/lambda_extras2.go:70::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /2018-10-31/layers/{layer}/versions/{version}/policy` | ✓ `simulators/aws/lambda_extras2.go:73::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /2018-10-31/layers/{layer}/versions/{version}/policy` | ✓ `simulators/aws/lambda_extras2.go:74::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /2018-10-31/layers/{layer}/versions/{version}/policy/{statement}` | ✓ `simulators/aws/lambda_extras2.go:75::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /2015-03-31/functions/{name}/configuration` | ✓ `simulators/aws/lambda_extras3.go:45::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /2015-03-31/functions/{name}/code` | ✓ `simulators/aws/lambda_extras3.go:46::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /2025-11-30/functions/{name}/function-scaling-config` | ✓ `simulators/aws/lambda_extras3.go:49::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /2025-11-30/functions/{name}/function-scaling-config` | ✓ `simulators/aws/lambda_extras3.go:50::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /2020-04-22/code-signing-configs/{arn}/functions` | ✓ `simulators/aws/lambda_extras3.go:55::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /2025-11-30/capacity-providers` | ✓ `simulators/aws/lambda_extras3.go:58::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /2025-11-30/capacity-providers` | ✓ `simulators/aws/lambda_extras3.go:59::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /2025-11-30/capacity-providers/{cpname}` | ✓ `simulators/aws/lambda_extras3.go:60::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /2025-11-30/capacity-providers/{cpname}` | ✓ `simulators/aws/lambda_extras3.go:61::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /2025-11-30/capacity-providers/{cpname}` | ✓ `simulators/aws/lambda_extras3.go:62::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /2025-11-30/capacity-providers/{cpname}/function-versions` | ✓ `simulators/aws/lambda_extras3.go:63::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /2014-11-13/functions/{name}/invoke-async` | ✓ `simulators/aws/lambda_extras3.go:66::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /2021-11-15/functions/{name}/response-streaming-invocations` | ✓ `simulators/aws/lambda_extras3.go:67::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /2025-12-01/functions/{name}/durable-executions` | ✓ `simulators/aws/lambda_extras3.go:73::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /2025-12-01/durable-executions/{arn}` | ✓ `simulators/aws/lambda_extras3.go:74::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /2025-12-01/durable-executions/{arn}/checkpoint` | ✓ `simulators/aws/lambda_extras3.go:75::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /2025-12-01/durable-executions/{arn}/history` | ✓ `simulators/aws/lambda_extras3.go:76::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /2025-12-01/durable-executions/{arn}/state` | ✓ `simulators/aws/lambda_extras3.go:77::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /2025-12-01/durable-executions/{arn}/stop` | ✓ `simulators/aws/lambda_extras3.go:78::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /2025-12-01/durable-execution-callbacks/{cbid}/succeed` | ✓ `simulators/aws/lambda_extras3.go:79::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /2025-12-01/durable-execution-callbacks/{cbid}/fail` | ✓ `simulators/aws/lambda_extras3.go:80::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /2025-12-01/durable-execution-callbacks/{cbid}/heartbeat` | ✓ `simulators/aws/lambda_extras3.go:81::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
