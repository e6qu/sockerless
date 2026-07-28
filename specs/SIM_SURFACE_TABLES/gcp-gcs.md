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
| `POST /storage/v1/b` | ✓ `simulators/gcp/gcs.go:621::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /storage/v1/b/{bucket}` | ✓ `simulators/gcp/gcs.go:664::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /storage/v1/b/{bucket}` | ✓ `simulators/gcp/gcs.go:706::patchBucket` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /storage/v1/b/{bucket}` | ✓ `simulators/gcp/gcs.go:707::patchBucket` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /storage/v1/b/{bucket}/storageLayout` | ✓ `simulators/gcp/gcs.go:710::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /storage/v1/b/{bucket}` | ✓ `simulators/gcp/gcs.go:738::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /storage/v1/b` | ✓ `simulators/gcp/gcs.go:757::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /storage/v1/b/{bucket}/o` | ✓ `simulators/gcp/gcs.go:780::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /storage/v1/b/{bucket}/o/{object...}` | ✓ `simulators/gcp/gcs.go:857::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /storage/v1/b/{bucket}/o/{object...}` | ✓ `simulators/gcp/gcs.go:911::patchObject` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /storage/v1/b/{bucket}/o/{object...}` | ✓ `simulators/gcp/gcs.go:912::patchObject` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /storage/v1/b/{bucket}/o/{object...}` | ✓ `simulators/gcp/gcs.go:915::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /upload/storage/v1/b/{bucket}/o` | ✓ `simulators/gcp/gcs.go:932::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /upload/storage/v1/b/{bucket}/o` | ✓ `simulators/gcp/gcs.go:944::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /storage/v1/b/{bucket}/o/{destObject...}` | ✓ `simulators/gcp/gcs.go:1115::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /download/storage/v1/b/{bucket}/o/{object...}` | ✓ `simulators/gcp/gcs.go:1236::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /storage/v1/b/{bucket}/acl` | ✓ `simulators/gcp/gcs.go:1603::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /storage/v1/b/{bucket}/acl/{entity}` | ✓ `simulators/gcp/gcs.go:1616::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /storage/v1/b/{bucket}/acl` | ✓ `simulators/gcp/gcs.go:1644::insert` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /storage/v1/b/{bucket}/acl/{entity}` | ✓ `simulators/gcp/gcs.go:1664::update` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /storage/v1/b/{bucket}/acl/{entity}` | ✓ `simulators/gcp/gcs.go:1665::update` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /storage/v1/b/{bucket}/acl/{entity}` | ✓ `simulators/gcp/gcs.go:1667::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /storage/v1/b/{bucket}/defaultObjectAcl` | ✓ `simulators/gcp/gcs.go:1696::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /storage/v1/b/{bucket}/defaultObjectAcl/{entity}` | ✓ `simulators/gcp/gcs.go:1709::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /storage/v1/b/{bucket}/defaultObjectAcl` | ✓ `simulators/gcp/gcs.go:1719::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /storage/v1/b/{bucket}/defaultObjectAcl/{entity}` | ✓ `simulators/gcp/gcs.go:1756::update` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /storage/v1/b/{bucket}/defaultObjectAcl/{entity}` | ✓ `simulators/gcp/gcs.go:1757::update` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /storage/v1/b/{bucket}/defaultObjectAcl/{entity}` | ✓ `simulators/gcp/gcs.go:1759::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /storage/v1/b/{bucket}/folders` | ✓ `simulators/gcp/gcs.go:1787::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /storage/v1/b/{bucket}/folders/{folder}` | ✓ `simulators/gcp/gcs.go:1812::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /storage/v1/b/{bucket}/folders` | ✓ `simulators/gcp/gcs.go:1822::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /storage/v1/b/{bucket}/folders/{folder}` | ✓ `simulators/gcp/gcs.go:1845::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /storage/v1/b/{bucket}/folders/{folder}/deleteRecursive` | ✓ `simulators/gcp/gcs.go:1857::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /storage/v1/b/{bucket}/folders/{sourceFolder}/renameTo/folders/{destinationFolder}` | ✓ `simulators/gcp/gcs.go:1872::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /storage/v1/b/{bucket}/managedFolders` | ✓ `simulators/gcp/gcs.go:1908::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /storage/v1/b/{bucket}/managedFolders/{managedFolder}` | ✓ `simulators/gcp/gcs.go:1933::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /storage/v1/b/{bucket}/managedFolders` | ✓ `simulators/gcp/gcs.go:1943::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /storage/v1/b/{bucket}/managedFolders/{managedFolder}` | ✓ `simulators/gcp/gcs.go:1966::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /storage/v1/b/{bucket}/managedFolders/{managedFolder}/iam` | ✓ `simulators/gcp/gcs.go:1980::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /storage/v1/b/{bucket}/managedFolders/{managedFolder}/iam` | ✓ `simulators/gcp/gcs.go:1991::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /storage/v1/b/{bucket}/managedFolders/{managedFolder}/iam/testPermissions` | ✓ `simulators/gcp/gcs.go:2007::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /storage/v1/b/{bucket}/notificationConfigs` | ✓ `simulators/gcp/gcs.go:2017::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /storage/v1/b/{bucket}/notificationConfigs/{notification}` | ✓ `simulators/gcp/gcs.go:2030::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /storage/v1/b/{bucket}/notificationConfigs` | ✓ `simulators/gcp/gcs.go:2040::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /storage/v1/b/{bucket}/notificationConfigs/{notification}` | ✓ `simulators/gcp/gcs.go:2066::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /storage/v1/projects/{projectId}/serviceAccount` | ✓ `simulators/gcp/gcs.go:2079::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /storage/v1/projects/{projectId}/hmacKeys` | ✓ `simulators/gcp/gcs.go:2087::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /storage/v1/projects/{projectId}/hmacKeys/{accessId}` | ✓ `simulators/gcp/gcs.go:2119::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /storage/v1/projects/{projectId}/hmacKeys` | ✓ `simulators/gcp/gcs.go:2129::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /storage/v1/projects/{projectId}/hmacKeys/{accessId}` | ✓ `simulators/gcp/gcs.go:2160::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /storage/v1/projects/{projectId}/hmacKeys/{accessId}` | ✓ `simulators/gcp/gcs.go:2180::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /storage/v1/b/{bucket}/anywhereCaches` | ✓ `simulators/gcp/gcs.go:2202::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /storage/v1/b/{bucket}/anywhereCaches/{anywhereCacheId}` | ✓ `simulators/gcp/gcs.go:2224::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /storage/v1/b/{bucket}/anywhereCaches` | ✓ `simulators/gcp/gcs.go:2236::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /storage/v1/b/{bucket}/anywhereCaches/{anywhereCacheId}` | ✓ `simulators/gcp/gcs.go:2265::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /storage/v1/b/{bucket}/anywhereCaches/{anywhereCacheId}/pause` | ✓ `simulators/gcp/gcs.go:2302::stateVerb` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /storage/v1/b/{bucket}/anywhereCaches/{anywhereCacheId}/resume` | ✓ `simulators/gcp/gcs.go:2303::stateVerb` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /storage/v1/b/{bucket}/anywhereCaches/{anywhereCacheId}/disable` | ✓ `simulators/gcp/gcs.go:2304::stateVerb` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /storage/v1/b/{bucket}/iam/testPermissions` | ✓ `simulators/gcp/gcs.go:2329::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /storage/v1/b/{bucket}/lockRetentionPolicy` | ✓ `simulators/gcp/gcs.go:2343::returnBucket` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /storage/v1/b/{bucket}/restore` | ✓ `simulators/gcp/gcs.go:2344::returnBucket` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /storage/v1/b/{bucket}/relocate` | ✓ `simulators/gcp/gcs.go:2347::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /storage/v1/b/{bucket}/operations` | ✓ `simulators/gcp/gcs.go:2358::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /storage/v1/b/{bucket}/operations/{operationId}` | ✓ `simulators/gcp/gcs.go:2368::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /storage/v1/b/{bucket}/operations/{operationId}/cancel` | ✓ `simulators/gcp/gcs.go:2374::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /storage/v1/b/{bucket}/operations/{operationId}/advanceRelocateBucket` | ✓ `simulators/gcp/gcs.go:2377::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /storage/v1/channels/stop` | ✓ `simulators/gcp/gcs.go:2383::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /storage/v1/b/{bucket}/o` | ✓ `simulators/gcp/gcs.go:2390::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
