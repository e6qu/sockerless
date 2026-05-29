# Sim surface — gcp-sqladmin

Surface registered in `simulators/gcp/sqladmin.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with a BUG / deferred-subtask reference; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no terraform-provider resource for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `POST /v1/projects/{project}/instances` | ✓ `simulators/gcp/sqladmin.go:104::handleSQLInsertInstance` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /v1/projects/{project}/instances/{instance}` | ✓ `simulators/gcp/sqladmin.go:105::handleSQLGetInstance` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /v1/projects/{project}/instances` | ✓ `simulators/gcp/sqladmin.go:106::handleSQLListInstances` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `PATCH /v1/projects/{project}/instances/{instance}` | ✓ `simulators/gcp/sqladmin.go:107::handleSQLPatchInstance` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /v1/projects/{project}/instances/{instance}` | ✓ `simulators/gcp/sqladmin.go:108::handleSQLDeleteInstance` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /v1/projects/{project}/instances/{instance}/databases` | ✓ `simulators/gcp/sqladmin.go:110::handleSQLInsertDatabase` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /v1/projects/{project}/instances/{instance}/databases` | ✓ `simulators/gcp/sqladmin.go:111::handleSQLListDatabases` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /v1/projects/{project}/instances/{instance}/databases/{database}` | ✓ `simulators/gcp/sqladmin.go:112::handleSQLDeleteDatabase` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /v1/projects/{project}/instances/{instance}/users` | ✓ `simulators/gcp/sqladmin.go:114::handleSQLInsertUser` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /v1/projects/{project}/instances/{instance}/users` | ✓ `simulators/gcp/sqladmin.go:115::handleSQLListUsers` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /v1/projects/{project}/instances/{instance}/backupRuns` | ✓ `simulators/gcp/sqladmin.go:117::handleSQLInsertBackupRun` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /v1/projects/{project}/instances/{instance}/backupRuns` | ✓ `simulators/gcp/sqladmin.go:118::handleSQLListBackupRuns` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /v1/projects/{project}/instances/{instance}/backupRuns/{id}` | ✓ `simulators/gcp/sqladmin.go:119::handleSQLGetBackupRun` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /v1/projects/{project}/instances/{instance}/backupRuns/{id}` | ✓ `simulators/gcp/sqladmin.go:120::handleSQLDeleteBackupRun` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /v1/projects/{project}/instances/{instance}/clone` | ✓ `simulators/gcp/sqladmin.go:121::handleSQLCloneInstance` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |

## Open subtasks staged forward

- sdk-test / tf-test columns are ✗-with-deferral for every row above. Each subsequent surface-touching PR fills in the column for the rows it covers; remaining ✗s are tracked under BUG-1159 (paged-iterator sweep) + BUG-1147 (tf-test parity sweep).
- Missing ops (not in HandleFunc but documented by the cloud provider) get ✗ rows added when a community-filed issue surfaces them or a periodic audit lands a sweep.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
