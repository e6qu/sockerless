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
| `GET /{$}` | ✓ `simulators/aws/s3.go:117::handleS3ListBuckets` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /{bucket}` | ✓ `simulators/aws/s3.go:118::handleS3PutBucketDispatch` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /{bucket}` | ✓ `simulators/aws/s3.go:119::handleS3DeleteBucketDispatch` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /{bucket}` | ✓ `simulators/aws/s3.go:120::handleS3GetOrHeadBucket` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /{bucket}/{key...}` | ✓ `simulators/aws/s3.go:121::handleS3PutObjectDispatch` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /{bucket}/{key...}` | ✓ `simulators/aws/s3.go:122::handleS3GetOrHeadObjectDispatch` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /{bucket}/{key...}` | ✓ `simulators/aws/s3.go:123::handleS3DeleteObjectDispatch` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /{bucket}/{key...}` | ✓ `simulators/aws/s3.go:128::handleS3PostObjectDispatch` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /{bucket}` | ✓ `simulators/aws/s3.go:129::handleS3PostBucketDispatch` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
