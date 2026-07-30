# Sim surface — aws-batch

Surface registered in `simulators/aws/batch.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `POST /v1/createcomputeenvironment` | ✓ `simulators/aws/batch.go:171::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/describecomputeenvironments` | ✓ `simulators/aws/batch.go:172::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/updatecomputeenvironment` | ✓ `simulators/aws/batch.go:173::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/deletecomputeenvironment` | ✓ `simulators/aws/batch.go:174::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/createjobqueue` | ✓ `simulators/aws/batch.go:176::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/describejobqueues` | ✓ `simulators/aws/batch.go:177::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/updatejobqueue` | ✓ `simulators/aws/batch.go:178::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/deletejobqueue` | ✓ `simulators/aws/batch.go:179::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/registerjobdefinition` | ✓ `simulators/aws/batch.go:181::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/describejobdefinitions` | ✓ `simulators/aws/batch.go:182::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/deregisterjobdefinition` | ✓ `simulators/aws/batch.go:183::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/submitjob` | ✓ `simulators/aws/batch.go:185::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/describejobs` | ✓ `simulators/aws/batch.go:186::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/listjobs` | ✓ `simulators/aws/batch.go:187::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/canceljob` | ✓ `simulators/aws/batch.go:188::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/terminatejob` | ✓ `simulators/aws/batch.go:189::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/createschedulingpolicy` | ✓ `simulators/aws/batch.go:191::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/describeschedulingpolicies` | ✓ `simulators/aws/batch.go:192::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/listschedulingpolicies` | ✓ `simulators/aws/batch.go:193::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/updateschedulingpolicy` | ✓ `simulators/aws/batch.go:194::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/deleteschedulingpolicy` | ✓ `simulators/aws/batch.go:195::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/createconsumableresource` | ✓ `simulators/aws/batch.go:198::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/describeconsumableresource` | ✓ `simulators/aws/batch.go:199::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/listconsumableresources` | ✓ `simulators/aws/batch.go:200::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/updateconsumableresource` | ✓ `simulators/aws/batch.go:201::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/deleteconsumableresource` | ✓ `simulators/aws/batch.go:202::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/listjobsbyconsumableresource` | ✓ `simulators/aws/batch.go:203::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/createserviceenvironment` | ✓ `simulators/aws/batch.go:206::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/describeserviceenvironments` | ✓ `simulators/aws/batch.go:207::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/updateserviceenvironment` | ✓ `simulators/aws/batch.go:208::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/deleteserviceenvironment` | ✓ `simulators/aws/batch.go:209::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/submitservicejob` | ✓ `simulators/aws/batch.go:212::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/describeservicejob` | ✓ `simulators/aws/batch.go:213::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/listservicejobs` | ✓ `simulators/aws/batch.go:214::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/terminateservicejob` | ✓ `simulators/aws/batch.go:215::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/updateservicejob` | ✓ `simulators/aws/batch.go:216::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/createquotashare` | ✓ `simulators/aws/batch.go:219::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/describequotashare` | ✓ `simulators/aws/batch.go:220::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/listquotashares` | ✓ `simulators/aws/batch.go:221::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/updatequotashare` | ✓ `simulators/aws/batch.go:222::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/deletequotashare` | ✓ `simulators/aws/batch.go:223::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/getjobqueuesnapshot` | ✓ `simulators/aws/batch.go:226::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/tags/{resourceArn}` | ✓ `simulators/aws/batch.go:229::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/tags/{resourceArn}` | ✓ `simulators/aws/batch.go:230::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v1/tags/{resourceArn}` | ✓ `simulators/aws/batch.go:231::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
