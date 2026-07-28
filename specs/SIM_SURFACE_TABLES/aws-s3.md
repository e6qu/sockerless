# Sim surface — aws-s3

Surface registered in `simulators/aws/s3.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `GET /{$}` | ✓ `simulators/aws/s3.go:167::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /{bucket}` | ✓ `simulators/aws/s3.go:168::cloudTrailRecordedRESTDynamic` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /{bucket}` | ✓ `simulators/aws/s3.go:169::cloudTrailRecordedRESTDynamic` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /{bucket}` | ✓ `simulators/aws/s3.go:170::cloudTrailRecordedRESTDynamic` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /{bucket}/{key...}` | ✓ `simulators/aws/s3.go:171::cloudTrailRecordedRESTDynamic` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /{bucket}/{key...}` | ✓ `simulators/aws/s3.go:172::cloudTrailRecordedRESTDynamic` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /{bucket}/{key...}` | ✓ `simulators/aws/s3.go:173::cloudTrailRecordedRESTDynamic` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /{bucket}/{key...}` | ✓ `simulators/aws/s3.go:178::cloudTrailRecordedRESTDynamic` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /{bucket}` | ✓ `simulators/aws/s3.go:179::cloudTrailRecordedRESTDynamic` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
