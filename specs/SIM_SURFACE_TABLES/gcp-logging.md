# Sim surface — gcp-logging

Surface registered in `simulators/gcp/logging.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with a BUG / deferred-subtask reference; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no terraform-provider resource for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `POST /v2/entries:list` | ✓ `simulators/gcp/logging.go:184::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /v2/entries:write` | ✓ `simulators/gcp/logging.go:197::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /v2/projects/{project}/sinks` | ✓ `simulators/gcp/logging.go:207::handleCreateLoggingSink` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /v2/projects/{project}/sinks` | ✓ `simulators/gcp/logging.go:208::handleListLoggingSinks` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /v2/projects/{project}/sinks/{sink}` | ✓ `simulators/gcp/logging.go:209::handleGetLoggingSink` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `PUT /v2/projects/{project}/sinks/{sink}` | ✓ `simulators/gcp/logging.go:210::handleUpdateLoggingSink` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `PATCH /v2/projects/{project}/sinks/{sink}` | ✓ `simulators/gcp/logging.go:211::handleUpdateLoggingSink` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /v2/projects/{project}/sinks/{sink}` | ✓ `simulators/gcp/logging.go:212::handleDeleteLoggingSink` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /v2/projects/{project}/metrics` | ✓ `simulators/gcp/logging.go:214::handleCreateLoggingMetric` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /v2/projects/{project}/metrics` | ✓ `simulators/gcp/logging.go:215::handleListLoggingMetrics` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /v2/projects/{project}/metrics/{metric}` | ✓ `simulators/gcp/logging.go:216::handleGetLoggingMetric` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `PUT /v2/projects/{project}/metrics/{metric}` | ✓ `simulators/gcp/logging.go:217::handleUpdateLoggingMetric` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `PATCH /v2/projects/{project}/metrics/{metric}` | ✓ `simulators/gcp/logging.go:218::handleUpdateLoggingMetric` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /v2/projects/{project}/metrics/{metric}` | ✓ `simulators/gcp/logging.go:219::handleDeleteLoggingMetric` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |

## Open subtasks staged forward

- sdk-test / tf-test columns are ✗-with-deferral for every row above. Each subsequent surface-touching PR fills in the column for the rows it covers; remaining ✗s are tracked under BUG-1159 (paged-iterator sweep) + BUG-1147 (tf-test parity sweep).
- Missing ops (not in HandleFunc but documented by the cloud provider) get ✗ rows added when a community-filed issue surfaces them or a periodic audit lands a sweep.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
