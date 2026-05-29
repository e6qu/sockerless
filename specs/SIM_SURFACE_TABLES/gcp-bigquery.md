# Sim surface — gcp-bigquery

Surface registered in `simulators/gcp/bigquery.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with a BUG / deferred-subtask reference; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no terraform-provider resource for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `POST /bigquery/v2/projects/{project}/datasets` | ✓ `simulators/gcp/bigquery.go:126::handleBQInsertDataset` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /bigquery/v2/projects/{project}/datasets` | ✓ `simulators/gcp/bigquery.go:127::handleBQListDatasets` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /bigquery/v2/projects/{project}/datasets/{dataset}` | ✓ `simulators/gcp/bigquery.go:128::handleBQGetDataset` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `PATCH /bigquery/v2/projects/{project}/datasets/{dataset}` | ✓ `simulators/gcp/bigquery.go:129::handleBQPatchDataset` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `PUT /bigquery/v2/projects/{project}/datasets/{dataset}` | ✓ `simulators/gcp/bigquery.go:130::handleBQPatchDataset` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /bigquery/v2/projects/{project}/datasets/{dataset}` | ✓ `simulators/gcp/bigquery.go:131::handleBQDeleteDataset` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /bigquery/v2/projects/{project}/datasets/{dataset}/tables` | ✓ `simulators/gcp/bigquery.go:133::handleBQInsertTable` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /bigquery/v2/projects/{project}/datasets/{dataset}/tables` | ✓ `simulators/gcp/bigquery.go:134::handleBQListTables` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /bigquery/v2/projects/{project}/datasets/{dataset}/tables/{table}` | ✓ `simulators/gcp/bigquery.go:135::handleBQGetTable` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `PATCH /bigquery/v2/projects/{project}/datasets/{dataset}/tables/{table}` | ✓ `simulators/gcp/bigquery.go:136::handleBQPatchTable` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `PUT /bigquery/v2/projects/{project}/datasets/{dataset}/tables/{table}` | ✓ `simulators/gcp/bigquery.go:137::handleBQPatchTable` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /bigquery/v2/projects/{project}/datasets/{dataset}/tables/{table}` | ✓ `simulators/gcp/bigquery.go:138::handleBQDeleteTable` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /bigquery/v2/projects/{project}/datasets/{dataset}/tables/{table}/insertAll` | ✓ `simulators/gcp/bigquery.go:140::handleBQInsertAll` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /bigquery/v2/projects/{project}/datasets/{dataset}/tables/{table}/data` | ✓ `simulators/gcp/bigquery.go:141::handleBQTableDataList` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /bigquery/v2/projects/{project}/queries` | ✓ `simulators/gcp/bigquery.go:143::handleBQQuery` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /bigquery/v2/projects/{project}/jobs` | ✓ `simulators/gcp/bigquery.go:144::handleBQInsertJob` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /bigquery/v2/projects/{project}/jobs` | ✓ `simulators/gcp/bigquery.go:145::handleBQListJobs` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /bigquery/v2/projects/{project}/jobs/{job}` | ✓ `simulators/gcp/bigquery.go:146::handleBQGetJob` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /bigquery/v2/projects/{project}/queries/{job}` | ✓ `simulators/gcp/bigquery.go:147::handleBQGetQueryResults` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |

## Open subtasks staged forward

- sdk-test / tf-test columns are ✗-with-deferral for every row above. Each subsequent surface-touching PR fills in the column for the rows it covers; remaining ✗s are tracked under BUG-1159 (paged-iterator sweep) + BUG-1147 (tf-test parity sweep).
- Missing ops (not in HandleFunc but documented by the cloud provider) get ✗ rows added when a community-filed issue surfaces them or a periodic audit lands a sweep.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
