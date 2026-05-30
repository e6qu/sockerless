# Sim surface — gcp-logging

Surface registered in `simulators/gcp/logging.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `POST /v2/entries:list` | ✓ `simulators/gcp/logging.go:184::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/entries:write` | ✓ `simulators/gcp/logging.go:197::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/sinks` | ✓ `simulators/gcp/logging.go:207::handleCreateLoggingSink` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/sinks` | ✓ `simulators/gcp/logging.go:208::handleListLoggingSinks` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/sinks/{sink}` | ✓ `simulators/gcp/logging.go:209::handleGetLoggingSink` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /v2/projects/{project}/sinks/{sink}` | ✓ `simulators/gcp/logging.go:210::handleUpdateLoggingSink` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v2/projects/{project}/sinks/{sink}` | ✓ `simulators/gcp/logging.go:211::handleUpdateLoggingSink` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/projects/{project}/sinks/{sink}` | ✓ `simulators/gcp/logging.go:212::handleDeleteLoggingSink` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/metrics` | ✓ `simulators/gcp/logging.go:214::handleCreateLoggingMetric` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/metrics` | ✓ `simulators/gcp/logging.go:215::handleListLoggingMetrics` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/metrics/{metric}` | ✓ `simulators/gcp/logging.go:216::handleGetLoggingMetric` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /v2/projects/{project}/metrics/{metric}` | ✓ `simulators/gcp/logging.go:217::handleUpdateLoggingMetric` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v2/projects/{project}/metrics/{metric}` | ✓ `simulators/gcp/logging.go:218::handleUpdateLoggingMetric` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/projects/{project}/metrics/{metric}` | ✓ `simulators/gcp/logging.go:219::handleDeleteLoggingMetric` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
