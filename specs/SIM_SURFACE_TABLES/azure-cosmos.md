# Sim surface — azure-cosmos

Surface registered in `simulators/azure/cosmos.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `POST /dbs` | ✓ `simulators/azure/cosmos.go:100::handleCosmosDataCreateDB` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /dbs` | ✓ `simulators/azure/cosmos.go:101::handleCosmosDataListDBs` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /dbs/{database}` | ✓ `simulators/azure/cosmos.go:102::handleCosmosDataGetDB` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /dbs/{database}` | ✓ `simulators/azure/cosmos.go:103::handleCosmosDataDeleteDB` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /dbs/{database}/colls` | ✓ `simulators/azure/cosmos.go:104::handleCosmosDataCreateColl` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /dbs/{database}/colls` | ✓ `simulators/azure/cosmos.go:105::handleCosmosDataListColls` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /dbs/{database}/colls/{container}` | ✓ `simulators/azure/cosmos.go:106::handleCosmosDataGetColl` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /dbs/{database}/colls/{container}` | ✓ `simulators/azure/cosmos.go:107::handleCosmosDataDeleteColl` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /dbs/{database}/colls/{container}/docs` | ✓ `simulators/azure/cosmos.go:108::handleCosmosDataCreateOrQueryDoc` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /dbs/{database}/colls/{container}/docs` | ✓ `simulators/azure/cosmos.go:109::handleCosmosDataListDocs` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /dbs/{database}/colls/{container}/docs/{doc}` | ✓ `simulators/azure/cosmos.go:110::handleCosmosDataGetDoc` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /dbs/{database}/colls/{container}/docs/{doc}` | ✓ `simulators/azure/cosmos.go:111::handleCosmosDataReplaceDoc` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /dbs/{database}/colls/{container}/docs/{doc}` | ✓ `simulators/azure/cosmos.go:112::handleCosmosDataDeleteDoc` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
