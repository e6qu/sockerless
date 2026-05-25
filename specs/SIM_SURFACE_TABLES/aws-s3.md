# Sim surface — aws-s3

Surface registered in `simulators/aws/s3.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with a BUG / deferred-subtask reference; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no terraform-provider resource for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `GET /{$}` | ✓ `simulators/aws/s3.go:117::handleS3ListBuckets` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `PUT /{bucket}` | ✓ `simulators/aws/s3.go:118::handleS3PutBucketDispatch` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /{bucket}` | ✓ `simulators/aws/s3.go:119::handleS3DeleteBucketDispatch` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /{bucket}` | ✓ `simulators/aws/s3.go:120::handleS3GetOrHeadBucket` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `PUT /{bucket}/{key...}` | ✓ `simulators/aws/s3.go:121::handleS3PutObjectDispatch` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /{bucket}/{key...}` | ✓ `simulators/aws/s3.go:122::handleS3GetOrHeadObjectDispatch` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /{bucket}/{key...}` | ✓ `simulators/aws/s3.go:123::handleS3DeleteObjectDispatch` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /{bucket}/{key...}` | ✓ `simulators/aws/s3.go:128::handleS3PostObjectDispatch` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /{bucket}` | ✓ `simulators/aws/s3.go:129::handleS3PostBucketDispatch` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |

## Open subtasks staged forward

- sdk-test / tf-test columns are ✗-with-deferral for every row above. Each subsequent surface-touching PR fills in the column for the rows it covers; remaining ✗s are tracked under BUG-1159 (paged-iterator sweep) + BUG-1147 (tf-test parity sweep).
- Missing ops (not in HandleFunc but documented by the cloud provider) get ✗ rows added when a community-filed issue surfaces them or a periodic audit lands a sweep.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
