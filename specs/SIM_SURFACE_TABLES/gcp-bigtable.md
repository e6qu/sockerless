# Sim surface — gcp-bigtable

Surface registered in `simulators/gcp/bigtable.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `GET /v2/operations/projects/{project}/operations` | ✓ `simulators/gcp/bigtable.go:69::handleBigtableListOperations` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances` | ✓ `simulators/gcp/bigtable.go:72::handleBigtableCreateInstance` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances` | ✓ `simulators/gcp/bigtable.go:73::handleBigtableListInstances` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances/{instanceAction}` | ✓ `simulators/gcp/bigtable.go:76::handleBigtableInstanceAction` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}` | ✓ `simulators/gcp/bigtable.go:77::handleBigtableGetInstance` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v2/projects/{project}/instances/{instance}` | ✓ `simulators/gcp/bigtable.go:78::handleBigtablePartialUpdateInstance` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /v2/projects/{project}/instances/{instance}` | ✓ `simulators/gcp/bigtable.go:79::handleBigtableReplaceInstance` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/projects/{project}/instances/{instance}` | ✓ `simulators/gcp/bigtable.go:80::handleBigtableDeleteInstance` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances/{instance}/clusters` | ✓ `simulators/gcp/bigtable.go:83::handleBigtableCreateCluster` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/clusters` | ✓ `simulators/gcp/bigtable.go:84::handleBigtableListClusters` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/clusters/{cluster}` | ✓ `simulators/gcp/bigtable.go:85::handleBigtableGetCluster` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /v2/projects/{project}/instances/{instance}/clusters/{cluster}` | ✓ `simulators/gcp/bigtable.go:86::handleBigtableUpdateCluster` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v2/projects/{project}/instances/{instance}/clusters/{cluster}` | ✓ `simulators/gcp/bigtable.go:87::handleBigtablePartialUpdateCluster` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/projects/{project}/instances/{instance}/clusters/{cluster}` | ✓ `simulators/gcp/bigtable.go:88::handleBigtableDeleteCluster` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/clusters/{cluster}/hotTablets` | ✓ `simulators/gcp/bigtable.go:91::handleBigtableListHotTablets` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/clusters/{cluster}/memoryLayer` | ✓ `simulators/gcp/bigtable.go:92::handleBigtableGetMemoryLayer` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/clusters/{cluster}/memoryLayers` | ✓ `simulators/gcp/bigtable.go:93::handleBigtableListMemoryLayers` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances/{instance}/clusters/{cluster}/backups` | ✓ `simulators/gcp/bigtable.go:98::handleBigtableCreateBackup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/clusters/{cluster}/backups` | ✓ `simulators/gcp/bigtable.go:99::handleBigtableListBackups` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances/{instance}/clusters/{cluster}/{backupsColl}` | ✓ `simulators/gcp/bigtable.go:100::handleBigtableBackupCollectionAction` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances/{instance}/clusters/{cluster}/backups/{backupAction}` | ✓ `simulators/gcp/bigtable.go:102::handleBigtableBackupItemAction` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/clusters/{cluster}/backups/{backup}` | ✓ `simulators/gcp/bigtable.go:103::handleBigtableGetBackup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v2/projects/{project}/instances/{instance}/clusters/{cluster}/backups/{backup}` | ✓ `simulators/gcp/bigtable.go:104::handleBigtablePatchBackup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/projects/{project}/instances/{instance}/clusters/{cluster}/backups/{backup}` | ✓ `simulators/gcp/bigtable.go:105::handleBigtableDeleteBackup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances/{instance}/appProfiles` | ✓ `simulators/gcp/bigtable.go:108::handleBigtableCreateAppProfile` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/appProfiles` | ✓ `simulators/gcp/bigtable.go:109::handleBigtableListAppProfiles` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/appProfiles/{appProfile}` | ✓ `simulators/gcp/bigtable.go:110::handleBigtableGetAppProfile` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v2/projects/{project}/instances/{instance}/appProfiles/{appProfile}` | ✓ `simulators/gcp/bigtable.go:111::handleBigtablePatchAppProfile` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/projects/{project}/instances/{instance}/appProfiles/{appProfile}` | ✓ `simulators/gcp/bigtable.go:112::handleBigtableDeleteAppProfile` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances/{instance}/tables` | ✓ `simulators/gcp/bigtable.go:117::handleBigtableCreateTable` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/tables` | ✓ `simulators/gcp/bigtable.go:118::handleBigtableListTables` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances/{instance}/{tablesColl}` | ✓ `simulators/gcp/bigtable.go:119::handleBigtableTableCollectionAction` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances/{instance}/tables/{tableAction}` | ✓ `simulators/gcp/bigtable.go:122::handleBigtableTableAction` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/tables/{table}` | ✓ `simulators/gcp/bigtable.go:123::handleBigtableGetTable` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v2/projects/{project}/instances/{instance}/tables/{table}` | ✓ `simulators/gcp/bigtable.go:124::handleBigtablePatchTable` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/projects/{project}/instances/{instance}/tables/{table}` | ✓ `simulators/gcp/bigtable.go:125::handleBigtableDeleteTable` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances/{instance}/tables/{table}/authorizedViews` | ✓ `simulators/gcp/bigtable.go:128::handleBigtableCreateAuthView` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/tables/{table}/authorizedViews` | ✓ `simulators/gcp/bigtable.go:129::handleBigtableListAuthViews` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances/{instance}/tables/{table}/authorizedViews/{authViewAction}` | ✓ `simulators/gcp/bigtable.go:130::handleBigtableAuthViewItemAction` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/tables/{table}/authorizedViews/{authView}` | ✓ `simulators/gcp/bigtable.go:131::handleBigtableGetAuthView` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v2/projects/{project}/instances/{instance}/tables/{table}/authorizedViews/{authView}` | ✓ `simulators/gcp/bigtable.go:132::handleBigtablePatchAuthView` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/projects/{project}/instances/{instance}/tables/{table}/authorizedViews/{authView}` | ✓ `simulators/gcp/bigtable.go:133::handleBigtableDeleteAuthView` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances/{instance}/tables/{table}/schemaBundles` | ✓ `simulators/gcp/bigtable.go:136::handleBigtableCreateSchemaBundle` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/tables/{table}/schemaBundles` | ✓ `simulators/gcp/bigtable.go:137::handleBigtableListSchemaBundles` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances/{instance}/tables/{table}/schemaBundles/{schemaBundleAction}` | ✓ `simulators/gcp/bigtable.go:138::handleBigtableSchemaBundleItemAction` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/tables/{table}/schemaBundles/{schemaBundle}` | ✓ `simulators/gcp/bigtable.go:139::handleBigtableGetSchemaBundle` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v2/projects/{project}/instances/{instance}/tables/{table}/schemaBundles/{schemaBundle}` | ✓ `simulators/gcp/bigtable.go:140::handleBigtablePatchSchemaBundle` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/projects/{project}/instances/{instance}/tables/{table}/schemaBundles/{schemaBundle}` | ✓ `simulators/gcp/bigtable.go:141::handleBigtableDeleteSchemaBundle` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances/{instance}/logicalViews` | ✓ `simulators/gcp/bigtable.go:144::handleBigtableCreateLogicalView` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/logicalViews` | ✓ `simulators/gcp/bigtable.go:145::handleBigtableListLogicalViews` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances/{instance}/logicalViews/{logicalViewAction}` | ✓ `simulators/gcp/bigtable.go:146::handleBigtableLogicalViewItemAction` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/logicalViews/{logicalView}` | ✓ `simulators/gcp/bigtable.go:147::handleBigtableGetLogicalView` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v2/projects/{project}/instances/{instance}/logicalViews/{logicalView}` | ✓ `simulators/gcp/bigtable.go:148::handleBigtablePatchLogicalView` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/projects/{project}/instances/{instance}/logicalViews/{logicalView}` | ✓ `simulators/gcp/bigtable.go:149::handleBigtableDeleteLogicalView` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances/{instance}/materializedViews` | ✓ `simulators/gcp/bigtable.go:152::handleBigtableCreateMatView` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/materializedViews` | ✓ `simulators/gcp/bigtable.go:153::handleBigtableListMatViews` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances/{instance}/materializedViews/{matViewAction}` | ✓ `simulators/gcp/bigtable.go:154::handleBigtableMatViewItemAction` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/materializedViews/{matView}` | ✓ `simulators/gcp/bigtable.go:155::handleBigtableGetMatView` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v2/projects/{project}/instances/{instance}/materializedViews/{matView}` | ✓ `simulators/gcp/bigtable.go:156::handleBigtablePatchMatView` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/projects/{project}/instances/{instance}/materializedViews/{matView}` | ✓ `simulators/gcp/bigtable.go:157::handleBigtableDeleteMatView` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
