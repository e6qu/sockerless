# Sim surface — gcp-sqladmin

Surface registered in `simulators/gcp/sqladmin.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `POST /v1/projects/{project}/instances` | ✓ `simulators/gcp/sqladmin.go:104::handleSQLInsertInstance` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/instances/{instance}` | ✓ `simulators/gcp/sqladmin.go:105::handleSQLGetInstance` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/instances` | ✓ `simulators/gcp/sqladmin.go:106::handleSQLListInstances` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `PATCH /v1/projects/{project}/instances/{instance}` | ✓ `simulators/gcp/sqladmin.go:107::handleSQLPatchInstance` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `DELETE /v1/projects/{project}/instances/{instance}` | ✓ `simulators/gcp/sqladmin.go:108::handleSQLDeleteInstance` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `POST /v1/projects/{project}/instances/{instance}/databases` | ✓ `simulators/gcp/sqladmin.go:110::handleSQLInsertDatabase` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/instances/{instance}/databases` | ✓ `simulators/gcp/sqladmin.go:111::handleSQLListDatabases` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `DELETE /v1/projects/{project}/instances/{instance}/databases/{database}` | ✓ `simulators/gcp/sqladmin.go:112::handleSQLDeleteDatabase` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `POST /v1/projects/{project}/instances/{instance}/users` | ✓ `simulators/gcp/sqladmin.go:114::handleSQLInsertUser` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/instances/{instance}/users` | ✓ `simulators/gcp/sqladmin.go:115::handleSQLListUsers` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `POST /v1/projects/{project}/instances/{instance}/backupRuns` | ✓ `simulators/gcp/sqladmin.go:117::handleSQLInsertBackupRun` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/instances/{instance}/backupRuns` | ✓ `simulators/gcp/sqladmin.go:118::handleSQLListBackupRuns` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/instances/{instance}/backupRuns/{id}` | ✓ `simulators/gcp/sqladmin.go:119::handleSQLGetBackupRun` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `DELETE /v1/projects/{project}/instances/{instance}/backupRuns/{id}` | ✓ `simulators/gcp/sqladmin.go:120::handleSQLDeleteBackupRun` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `POST /v1/projects/{project}/instances/{instance}/clone` | ✓ `simulators/gcp/sqladmin.go:121::handleSQLCloneInstance` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
