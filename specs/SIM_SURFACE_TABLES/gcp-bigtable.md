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
| `GET /v2/operations/projects/{project}/operations` | ✓ `simulators/gcp/bigtable.go:83::handleBigtableListOperations` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances` | ✓ `simulators/gcp/bigtable.go:86::handleBigtableCreateInstance` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances` | ✓ `simulators/gcp/bigtable.go:87::handleBigtableListInstances` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances/{instanceAction}` | ✓ `simulators/gcp/bigtable.go:90::handleBigtableInstanceAction` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}` | ✓ `simulators/gcp/bigtable.go:91::handleBigtableGetInstance` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v2/projects/{project}/instances/{instance}` | ✓ `simulators/gcp/bigtable.go:92::handleBigtablePartialUpdateInstance` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /v2/projects/{project}/instances/{instance}` | ✓ `simulators/gcp/bigtable.go:93::handleBigtableReplaceInstance` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/projects/{project}/instances/{instance}` | ✓ `simulators/gcp/bigtable.go:94::handleBigtableDeleteInstance` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances/{instance}/clusters` | ✓ `simulators/gcp/bigtable.go:97::handleBigtableCreateCluster` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/clusters` | ✓ `simulators/gcp/bigtable.go:98::handleBigtableListClusters` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/clusters/{cluster}` | ✓ `simulators/gcp/bigtable.go:99::handleBigtableGetCluster` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /v2/projects/{project}/instances/{instance}/clusters/{cluster}` | ✓ `simulators/gcp/bigtable.go:100::handleBigtableUpdateCluster` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v2/projects/{project}/instances/{instance}/clusters/{cluster}` | ✓ `simulators/gcp/bigtable.go:101::handleBigtablePartialUpdateCluster` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/projects/{project}/instances/{instance}/clusters/{cluster}` | ✓ `simulators/gcp/bigtable.go:102::handleBigtableDeleteCluster` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/clusters/{cluster}/hotTablets` | ✓ `simulators/gcp/bigtable.go:105::handleBigtableListHotTablets` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/clusters/{cluster}/memoryLayer` | ✓ `simulators/gcp/bigtable.go:106::handleBigtableGetMemoryLayer` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v2/projects/{project}/instances/{instance}/clusters/{cluster}/memoryLayer` | ✓ `simulators/gcp/bigtable.go:107::handleBigtableUpdateMemoryLayer` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/clusters/{cluster}/memoryLayers` | ✓ `simulators/gcp/bigtable.go:108::handleBigtableListMemoryLayers` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances/{instance}/clusters/{cluster}/backups` | ✓ `simulators/gcp/bigtable.go:113::handleBigtableCreateBackup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/clusters/{cluster}/backups` | ✓ `simulators/gcp/bigtable.go:114::handleBigtableListBackups` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances/{instance}/clusters/{cluster}/{backupsColl}` | ✓ `simulators/gcp/bigtable.go:115::handleBigtableBackupCollectionAction` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances/{instance}/clusters/{cluster}/backups/{backupAction}` | ✓ `simulators/gcp/bigtable.go:117::handleBigtableBackupItemAction` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/clusters/{cluster}/backups/{backup}` | ✓ `simulators/gcp/bigtable.go:118::handleBigtableGetBackup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v2/projects/{project}/instances/{instance}/clusters/{cluster}/backups/{backup}` | ✓ `simulators/gcp/bigtable.go:119::handleBigtablePatchBackup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/projects/{project}/instances/{instance}/clusters/{cluster}/backups/{backup}` | ✓ `simulators/gcp/bigtable.go:120::handleBigtableDeleteBackup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances/{instance}/appProfiles` | ✓ `simulators/gcp/bigtable.go:123::handleBigtableCreateAppProfile` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/appProfiles` | ✓ `simulators/gcp/bigtable.go:124::handleBigtableListAppProfiles` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/appProfiles/{appProfile}` | ✓ `simulators/gcp/bigtable.go:125::handleBigtableGetAppProfile` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v2/projects/{project}/instances/{instance}/appProfiles/{appProfile}` | ✓ `simulators/gcp/bigtable.go:126::handleBigtablePatchAppProfile` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/projects/{project}/instances/{instance}/appProfiles/{appProfile}` | ✓ `simulators/gcp/bigtable.go:127::handleBigtableDeleteAppProfile` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances/{instance}/tables` | ✓ `simulators/gcp/bigtable.go:132::handleBigtableCreateTable` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/tables` | ✓ `simulators/gcp/bigtable.go:133::handleBigtableListTables` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances/{instance}/{tablesColl}` | ✓ `simulators/gcp/bigtable.go:134::handleBigtableTableCollectionAction` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances/{instance}/tables/{tableAction}` | ✓ `simulators/gcp/bigtable.go:137::handleBigtableTableAction` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/tables/{table}` | ✓ `simulators/gcp/bigtable.go:138::handleBigtableGetTable` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v2/projects/{project}/instances/{instance}/tables/{table}` | ✓ `simulators/gcp/bigtable.go:139::handleBigtablePatchTable` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/projects/{project}/instances/{instance}/tables/{table}` | ✓ `simulators/gcp/bigtable.go:140::handleBigtableDeleteTable` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances/{instance}/tables/{table}/authorizedViews` | ✓ `simulators/gcp/bigtable.go:143::handleBigtableCreateAuthView` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/tables/{table}/authorizedViews` | ✓ `simulators/gcp/bigtable.go:144::handleBigtableListAuthViews` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances/{instance}/tables/{table}/authorizedViews/{authViewAction}` | ✓ `simulators/gcp/bigtable.go:145::handleBigtableAuthViewItemAction` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/tables/{table}/authorizedViews/{authView}` | ✓ `simulators/gcp/bigtable.go:146::handleBigtableGetAuthView` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v2/projects/{project}/instances/{instance}/tables/{table}/authorizedViews/{authView}` | ✓ `simulators/gcp/bigtable.go:147::handleBigtablePatchAuthView` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/projects/{project}/instances/{instance}/tables/{table}/authorizedViews/{authView}` | ✓ `simulators/gcp/bigtable.go:148::handleBigtableDeleteAuthView` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances/{instance}/tables/{table}/schemaBundles` | ✓ `simulators/gcp/bigtable.go:151::handleBigtableCreateSchemaBundle` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/tables/{table}/schemaBundles` | ✓ `simulators/gcp/bigtable.go:152::handleBigtableListSchemaBundles` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances/{instance}/tables/{table}/schemaBundles/{schemaBundleAction}` | ✓ `simulators/gcp/bigtable.go:153::handleBigtableSchemaBundleItemAction` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/tables/{table}/schemaBundles/{schemaBundle}` | ✓ `simulators/gcp/bigtable.go:154::handleBigtableGetSchemaBundle` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v2/projects/{project}/instances/{instance}/tables/{table}/schemaBundles/{schemaBundle}` | ✓ `simulators/gcp/bigtable.go:155::handleBigtablePatchSchemaBundle` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/projects/{project}/instances/{instance}/tables/{table}/schemaBundles/{schemaBundle}` | ✓ `simulators/gcp/bigtable.go:156::handleBigtableDeleteSchemaBundle` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances/{instance}/logicalViews` | ✓ `simulators/gcp/bigtable.go:159::handleBigtableCreateLogicalView` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/logicalViews` | ✓ `simulators/gcp/bigtable.go:160::handleBigtableListLogicalViews` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances/{instance}/logicalViews/{logicalViewAction}` | ✓ `simulators/gcp/bigtable.go:161::handleBigtableLogicalViewItemAction` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/logicalViews/{logicalView}` | ✓ `simulators/gcp/bigtable.go:162::handleBigtableGetLogicalView` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v2/projects/{project}/instances/{instance}/logicalViews/{logicalView}` | ✓ `simulators/gcp/bigtable.go:163::handleBigtablePatchLogicalView` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/projects/{project}/instances/{instance}/logicalViews/{logicalView}` | ✓ `simulators/gcp/bigtable.go:164::handleBigtableDeleteLogicalView` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances/{instance}/materializedViews` | ✓ `simulators/gcp/bigtable.go:167::handleBigtableCreateMatView` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/materializedViews` | ✓ `simulators/gcp/bigtable.go:168::handleBigtableListMatViews` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances/{instance}/materializedViews/{matViewAction}` | ✓ `simulators/gcp/bigtable.go:169::handleBigtableMatViewItemAction` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/materializedViews/{matView}` | ✓ `simulators/gcp/bigtable.go:170::handleBigtableGetMatView` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v2/projects/{project}/instances/{instance}/materializedViews/{matView}` | ✓ `simulators/gcp/bigtable.go:171::handleBigtablePatchMatView` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/projects/{project}/instances/{instance}/materializedViews/{matView}` | ✓ `simulators/gcp/bigtable.go:172::handleBigtableDeleteMatView` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
