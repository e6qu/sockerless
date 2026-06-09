# Sim surface — gcp-bigtable

Surface registered in `simulators/gcp/bigtable.go`.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `POST /v2/projects/{project}/instances` | ✓ `handleBigtableCreateInstance` | ✓ (direct; see coverage matrix) | tracked BUG-1585 | n/a | Terraform provider Bigtable Admin calls bypass the REST custom endpoint. |
| `GET /v2/projects/{project}/instances` | ✓ `handleBigtableListInstances` | ✓ (direct; see coverage matrix) | tracked BUG-1585 | ✓ | |
| `GET /v2/projects/{project}/instances/{instance}` | ✓ `handleBigtableGetInstance` | ✓ (direct; see coverage matrix) | tracked BUG-1585 | n/a | |
| `DELETE /v2/projects/{project}/instances/{instance}` | ✓ `handleBigtableDeleteInstance` | ✓ (direct; see coverage matrix) | tracked BUG-1585 | n/a | |
| `GET /v2/operations/{operation}` | ✓ `registerOperations` | ✓ (direct; see coverage matrix) | tracked BUG-1585 | n/a | Bigtable Admin LRO collection. |
| `POST /v2/projects/{project}/instances/{instance}/clusters` | ✓ `handleBigtableCreateCluster` | ✓ (direct; see coverage matrix) | tracked BUG-1585 | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/clusters` | ✓ `handleBigtableListClusters` | ✓ (direct; see coverage matrix) | tracked BUG-1585 | ✓ | |
| `GET /v2/projects/{project}/instances/{instance}/clusters/{cluster}` | ✓ `handleBigtableGetCluster` | ✓ (direct; see coverage matrix) | tracked BUG-1585 | n/a | |
| `DELETE /v2/projects/{project}/instances/{instance}/clusters/{cluster}` | ✓ `handleBigtableDeleteCluster` | ✓ (direct; see coverage matrix) | tracked BUG-1585 | n/a | |
| `POST /v2/projects/{project}/instances/{instance}/tables` | ✓ `handleBigtableCreateTable` | ✓ (direct; see coverage matrix) | tracked BUG-1585 | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/tables` | ✓ `handleBigtableListTables` | ✓ (direct; see coverage matrix) | tracked BUG-1585 | ✓ | |
| `GET /v2/projects/{project}/instances/{instance}/tables/{table}` | ✓ `handleBigtableGetTable` | ✓ (direct; see coverage matrix) | tracked BUG-1585 | n/a | |
| `DELETE /v2/projects/{project}/instances/{instance}/tables/{table}` | ✓ `handleBigtableDeleteTable` | ✓ (direct; see coverage matrix) | tracked BUG-1585 | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
