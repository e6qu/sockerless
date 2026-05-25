# Sim surface — gcp-gcs

Surface registered in `simulators/gcp/gcs.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with a BUG / deferred-subtask reference; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no terraform-provider resource for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `POST /storage/v1/b` | ✓ `simulators/gcp/gcs.go:269::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /storage/v1/b/{bucket}` | ✓ `simulators/gcp/gcs.go:308::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /storage/v1/b/{bucket}` | ✓ `simulators/gcp/gcs.go:325::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /storage/v1/b` | ✓ `simulators/gcp/gcs.go:345::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /storage/v1/b/{bucket}/o` | ✓ `simulators/gcp/gcs.go:361::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /storage/v1/b/{bucket}/o/{object...}` | ✓ `simulators/gcp/gcs.go:447::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /storage/v1/b/{bucket}/o/{object...}` | ✓ `simulators/gcp/gcs.go:461::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `PUT /upload/storage/v1/b/{bucket}/o` | ✓ `simulators/gcp/gcs.go:477::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /upload/storage/v1/b/{bucket}/o` | ✓ `simulators/gcp/gcs.go:489::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /storage/v1/b/{bucket}/o/{destObject...}` | ✓ `simulators/gcp/gcs.go:696::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /download/storage/v1/b/{bucket}/o/{object...}` | ✓ `simulators/gcp/gcs.go:816::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |

## Open subtasks staged forward

- sdk-test / tf-test columns are ✗-with-deferral for every row above. Each subsequent surface-touching PR fills in the column for the rows it covers; remaining ✗s are tracked under BUG-1159 (paged-iterator sweep) + BUG-1147 (tf-test parity sweep).
- Missing ops (not in HandleFunc but documented by the cloud provider) get ✗ rows added when a community-filed issue surfaces them or a periodic audit lands a sweep.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
