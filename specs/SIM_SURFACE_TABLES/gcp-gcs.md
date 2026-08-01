# Sim surface — gcp-gcs

Surface registered in `simulators/gcp/gcs.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `POST /storage/v1/b` | ✓ `simulators/gcp/gcs.go:642::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /storage/v1/b/{bucket}` | ✓ `simulators/gcp/gcs.go:685::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /storage/v1/b/{bucket}` | ✓ `simulators/gcp/gcs.go:727::patchBucket` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /storage/v1/b/{bucket}` | ✓ `simulators/gcp/gcs.go:728::patchBucket` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /storage/v1/b/{bucket}/storageLayout` | ✓ `simulators/gcp/gcs.go:731::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /storage/v1/b/{bucket}` | ✓ `simulators/gcp/gcs.go:759::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /storage/v1/b` | ✓ `simulators/gcp/gcs.go:778::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /storage/v1/b/{bucket}/o` | ✓ `simulators/gcp/gcs.go:801::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /storage/v1/b/{bucket}/o/{object...}` | ✓ `simulators/gcp/gcs.go:878::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /storage/v1/b/{bucket}/o/{object...}` | ✓ `simulators/gcp/gcs.go:932::patchObject` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /storage/v1/b/{bucket}/o/{object...}` | ✓ `simulators/gcp/gcs.go:933::patchObject` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /storage/v1/b/{bucket}/o/{object...}` | ✓ `simulators/gcp/gcs.go:936::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /upload/storage/v1/b/{bucket}/o` | ✓ `simulators/gcp/gcs.go:953::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /upload/storage/v1/b/{bucket}/o` | ✓ `simulators/gcp/gcs.go:965::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /storage/v1/b/{bucket}/o/{destObject...}` | ✓ `simulators/gcp/gcs.go:1136::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /download/storage/v1/b/{bucket}/o/{object...}` | ✓ `simulators/gcp/gcs.go:1257::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /storage/v1/b/{bucket}/acl` | ✓ `simulators/gcp/gcs.go:1624::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /storage/v1/b/{bucket}/acl/{entity}` | ✓ `simulators/gcp/gcs.go:1637::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /storage/v1/b/{bucket}/acl` | ✓ `simulators/gcp/gcs.go:1665::insert` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /storage/v1/b/{bucket}/acl/{entity}` | ✓ `simulators/gcp/gcs.go:1685::update` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /storage/v1/b/{bucket}/acl/{entity}` | ✓ `simulators/gcp/gcs.go:1686::update` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /storage/v1/b/{bucket}/acl/{entity}` | ✓ `simulators/gcp/gcs.go:1688::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /storage/v1/b/{bucket}/defaultObjectAcl` | ✓ `simulators/gcp/gcs.go:1717::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /storage/v1/b/{bucket}/defaultObjectAcl/{entity}` | ✓ `simulators/gcp/gcs.go:1730::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /storage/v1/b/{bucket}/defaultObjectAcl` | ✓ `simulators/gcp/gcs.go:1740::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /storage/v1/b/{bucket}/defaultObjectAcl/{entity}` | ✓ `simulators/gcp/gcs.go:1777::update` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /storage/v1/b/{bucket}/defaultObjectAcl/{entity}` | ✓ `simulators/gcp/gcs.go:1778::update` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /storage/v1/b/{bucket}/defaultObjectAcl/{entity}` | ✓ `simulators/gcp/gcs.go:1780::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /storage/v1/b/{bucket}/folders` | ✓ `simulators/gcp/gcs.go:1808::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /storage/v1/b/{bucket}/folders/{folder}` | ✓ `simulators/gcp/gcs.go:1833::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /storage/v1/b/{bucket}/folders` | ✓ `simulators/gcp/gcs.go:1843::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /storage/v1/b/{bucket}/folders/{folder}` | ✓ `simulators/gcp/gcs.go:1866::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /storage/v1/b/{bucket}/folders/{folder}/deleteRecursive` | ✓ `simulators/gcp/gcs.go:1878::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /storage/v1/b/{bucket}/folders/{sourceFolder}/renameTo/folders/{destinationFolder}` | ✓ `simulators/gcp/gcs.go:1893::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /storage/v1/b/{bucket}/managedFolders` | ✓ `simulators/gcp/gcs.go:1929::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /storage/v1/b/{bucket}/managedFolders/{managedFolder}` | ✓ `simulators/gcp/gcs.go:1954::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /storage/v1/b/{bucket}/managedFolders` | ✓ `simulators/gcp/gcs.go:1964::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /storage/v1/b/{bucket}/managedFolders/{managedFolder}` | ✓ `simulators/gcp/gcs.go:1987::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /storage/v1/b/{bucket}/managedFolders/{managedFolder}/iam` | ✓ `simulators/gcp/gcs.go:2001::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /storage/v1/b/{bucket}/managedFolders/{managedFolder}/iam` | ✓ `simulators/gcp/gcs.go:2012::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /storage/v1/b/{bucket}/managedFolders/{managedFolder}/iam/testPermissions` | ✓ `simulators/gcp/gcs.go:2028::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /storage/v1/b/{bucket}/notificationConfigs` | ✓ `simulators/gcp/gcs.go:2038::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /storage/v1/b/{bucket}/notificationConfigs/{notification}` | ✓ `simulators/gcp/gcs.go:2051::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /storage/v1/b/{bucket}/notificationConfigs` | ✓ `simulators/gcp/gcs.go:2061::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /storage/v1/b/{bucket}/notificationConfigs/{notification}` | ✓ `simulators/gcp/gcs.go:2087::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /storage/v1/projects/{projectId}/serviceAccount` | ✓ `simulators/gcp/gcs.go:2100::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /storage/v1/projects/{projectId}/hmacKeys` | ✓ `simulators/gcp/gcs.go:2108::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /storage/v1/projects/{projectId}/hmacKeys/{accessId}` | ✓ `simulators/gcp/gcs.go:2140::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /storage/v1/projects/{projectId}/hmacKeys` | ✓ `simulators/gcp/gcs.go:2150::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /storage/v1/projects/{projectId}/hmacKeys/{accessId}` | ✓ `simulators/gcp/gcs.go:2181::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /storage/v1/projects/{projectId}/hmacKeys/{accessId}` | ✓ `simulators/gcp/gcs.go:2201::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /storage/v1/b/{bucket}/anywhereCaches` | ✓ `simulators/gcp/gcs.go:2223::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /storage/v1/b/{bucket}/anywhereCaches/{anywhereCacheId}` | ✓ `simulators/gcp/gcs.go:2245::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /storage/v1/b/{bucket}/anywhereCaches` | ✓ `simulators/gcp/gcs.go:2257::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /storage/v1/b/{bucket}/anywhereCaches/{anywhereCacheId}` | ✓ `simulators/gcp/gcs.go:2286::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /storage/v1/b/{bucket}/anywhereCaches/{anywhereCacheId}/pause` | ✓ `simulators/gcp/gcs.go:2323::stateVerb` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /storage/v1/b/{bucket}/anywhereCaches/{anywhereCacheId}/resume` | ✓ `simulators/gcp/gcs.go:2324::stateVerb` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /storage/v1/b/{bucket}/anywhereCaches/{anywhereCacheId}/disable` | ✓ `simulators/gcp/gcs.go:2325::stateVerb` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /storage/v1/b/{bucket}/iam/testPermissions` | ✓ `simulators/gcp/gcs.go:2350::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /storage/v1/b/{bucket}/lockRetentionPolicy` | ✓ `simulators/gcp/gcs.go:2364::returnBucket` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /storage/v1/b/{bucket}/restore` | ✓ `simulators/gcp/gcs.go:2365::returnBucket` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /storage/v1/b/{bucket}/relocate` | ✓ `simulators/gcp/gcs.go:2368::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /storage/v1/b/{bucket}/operations` | ✓ `simulators/gcp/gcs.go:2379::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /storage/v1/b/{bucket}/operations/{operationId}` | ✓ `simulators/gcp/gcs.go:2389::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /storage/v1/b/{bucket}/operations/{operationId}/cancel` | ✓ `simulators/gcp/gcs.go:2395::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /storage/v1/b/{bucket}/operations/{operationId}/advanceRelocateBucket` | ✓ `simulators/gcp/gcs.go:2398::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /storage/v1/channels/stop` | ✓ `simulators/gcp/gcs.go:2404::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /storage/v1/b/{bucket}/o` | ✓ `simulators/gcp/gcs.go:2411::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
