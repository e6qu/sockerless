# Sim surface — gcp-bigquery

Surface registered in `simulators/gcp/bigquery.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `POST /bigquery/v2/projects/{project}/datasets` | ✓ `simulators/gcp/bigquery.go:126::handleBQInsertDataset` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /bigquery/v2/projects/{project}/datasets` | ✓ `simulators/gcp/bigquery.go:127::handleBQListDatasets` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /bigquery/v2/projects/{project}/datasets/{dataset}` | ✓ `simulators/gcp/bigquery.go:128::handleBQGetDataset` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /bigquery/v2/projects/{project}/datasets/{dataset}` | ✓ `simulators/gcp/bigquery.go:129::handleBQPatchDataset` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /bigquery/v2/projects/{project}/datasets/{dataset}` | ✓ `simulators/gcp/bigquery.go:130::handleBQPatchDataset` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /bigquery/v2/projects/{project}/datasets/{dataset}` | ✓ `simulators/gcp/bigquery.go:131::handleBQDeleteDataset` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /bigquery/v2/projects/{project}/datasets/{dataset}/tables` | ✓ `simulators/gcp/bigquery.go:133::handleBQInsertTable` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /bigquery/v2/projects/{project}/datasets/{dataset}/tables` | ✓ `simulators/gcp/bigquery.go:134::handleBQListTables` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /bigquery/v2/projects/{project}/datasets/{dataset}/tables/{table}` | ✓ `simulators/gcp/bigquery.go:135::handleBQGetTable` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /bigquery/v2/projects/{project}/datasets/{dataset}/tables/{table}` | ✓ `simulators/gcp/bigquery.go:136::handleBQPatchTable` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /bigquery/v2/projects/{project}/datasets/{dataset}/tables/{table}` | ✓ `simulators/gcp/bigquery.go:137::handleBQPatchTable` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /bigquery/v2/projects/{project}/datasets/{dataset}/tables/{table}` | ✓ `simulators/gcp/bigquery.go:138::handleBQDeleteTable` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /bigquery/v2/projects/{project}/datasets/{dataset}/tables/{table}/insertAll` | ✓ `simulators/gcp/bigquery.go:140::handleBQInsertAll` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /bigquery/v2/projects/{project}/datasets/{dataset}/tables/{table}/data` | ✓ `simulators/gcp/bigquery.go:141::handleBQTableDataList` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /bigquery/v2/projects/{project}/queries` | ✓ `simulators/gcp/bigquery.go:143::handleBQQuery` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /bigquery/v2/projects/{project}/jobs` | ✓ `simulators/gcp/bigquery.go:144::handleBQInsertJob` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /bigquery/v2/projects/{project}/jobs` | ✓ `simulators/gcp/bigquery.go:145::handleBQListJobs` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /bigquery/v2/projects/{project}/jobs/{job}` | ✓ `simulators/gcp/bigquery.go:146::handleBQGetJob` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /bigquery/v2/projects/{project}/queries/{job}` | ✓ `simulators/gcp/bigquery.go:147::handleBQGetQueryResults` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
