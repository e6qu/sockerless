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
| `GET /v2/operations/projects/{project}/operations` | ✓ `simulators/gcp/bigtable.go:82::handleBigtableListOperations` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances` | ✓ `simulators/gcp/bigtable.go:85::handleBigtableCreateInstance` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances` | ✓ `simulators/gcp/bigtable.go:86::handleBigtableListInstances` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances/{instanceAction}` | ✓ `simulators/gcp/bigtable.go:89::handleBigtableInstanceAction` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}` | ✓ `simulators/gcp/bigtable.go:90::handleBigtableGetInstance` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v2/projects/{project}/instances/{instance}` | ✓ `simulators/gcp/bigtable.go:91::handleBigtablePartialUpdateInstance` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /v2/projects/{project}/instances/{instance}` | ✓ `simulators/gcp/bigtable.go:92::handleBigtableReplaceInstance` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/projects/{project}/instances/{instance}` | ✓ `simulators/gcp/bigtable.go:93::handleBigtableDeleteInstance` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances/{instance}/clusters` | ✓ `simulators/gcp/bigtable.go:96::handleBigtableCreateCluster` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/clusters` | ✓ `simulators/gcp/bigtable.go:97::handleBigtableListClusters` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/clusters/{cluster}` | ✓ `simulators/gcp/bigtable.go:98::handleBigtableGetCluster` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /v2/projects/{project}/instances/{instance}/clusters/{cluster}` | ✓ `simulators/gcp/bigtable.go:99::handleBigtableUpdateCluster` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v2/projects/{project}/instances/{instance}/clusters/{cluster}` | ✓ `simulators/gcp/bigtable.go:100::handleBigtablePartialUpdateCluster` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/projects/{project}/instances/{instance}/clusters/{cluster}` | ✓ `simulators/gcp/bigtable.go:101::handleBigtableDeleteCluster` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/clusters/{cluster}/hotTablets` | ✓ `simulators/gcp/bigtable.go:104::handleBigtableListHotTablets` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/clusters/{cluster}/memoryLayer` | ✓ `simulators/gcp/bigtable.go:105::handleBigtableGetMemoryLayer` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v2/projects/{project}/instances/{instance}/clusters/{cluster}/memoryLayer` | ✓ `simulators/gcp/bigtable.go:106::handleBigtableUpdateMemoryLayer` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/clusters/{cluster}/memoryLayers` | ✓ `simulators/gcp/bigtable.go:107::handleBigtableListMemoryLayers` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances/{instance}/clusters/{cluster}/backups` | ✓ `simulators/gcp/bigtable.go:112::handleBigtableCreateBackup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/clusters/{cluster}/backups` | ✓ `simulators/gcp/bigtable.go:113::handleBigtableListBackups` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances/{instance}/clusters/{cluster}/{backupsColl}` | ✓ `simulators/gcp/bigtable.go:114::handleBigtableBackupCollectionAction` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances/{instance}/clusters/{cluster}/backups/{backupAction}` | ✓ `simulators/gcp/bigtable.go:116::handleBigtableBackupItemAction` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/clusters/{cluster}/backups/{backup}` | ✓ `simulators/gcp/bigtable.go:117::handleBigtableGetBackup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v2/projects/{project}/instances/{instance}/clusters/{cluster}/backups/{backup}` | ✓ `simulators/gcp/bigtable.go:118::handleBigtablePatchBackup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/projects/{project}/instances/{instance}/clusters/{cluster}/backups/{backup}` | ✓ `simulators/gcp/bigtable.go:119::handleBigtableDeleteBackup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances/{instance}/appProfiles` | ✓ `simulators/gcp/bigtable.go:122::handleBigtableCreateAppProfile` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/appProfiles` | ✓ `simulators/gcp/bigtable.go:123::handleBigtableListAppProfiles` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/appProfiles/{appProfile}` | ✓ `simulators/gcp/bigtable.go:124::handleBigtableGetAppProfile` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v2/projects/{project}/instances/{instance}/appProfiles/{appProfile}` | ✓ `simulators/gcp/bigtable.go:125::handleBigtablePatchAppProfile` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/projects/{project}/instances/{instance}/appProfiles/{appProfile}` | ✓ `simulators/gcp/bigtable.go:126::handleBigtableDeleteAppProfile` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances/{instance}/tables` | ✓ `simulators/gcp/bigtable.go:131::handleBigtableCreateTable` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/tables` | ✓ `simulators/gcp/bigtable.go:132::handleBigtableListTables` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances/{instance}/{tablesColl}` | ✓ `simulators/gcp/bigtable.go:133::handleBigtableTableCollectionAction` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances/{instance}/tables/{tableAction}` | ✓ `simulators/gcp/bigtable.go:136::handleBigtableTableAction` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/tables/{table}` | ✓ `simulators/gcp/bigtable.go:137::handleBigtableGetTable` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v2/projects/{project}/instances/{instance}/tables/{table}` | ✓ `simulators/gcp/bigtable.go:138::handleBigtablePatchTable` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/projects/{project}/instances/{instance}/tables/{table}` | ✓ `simulators/gcp/bigtable.go:139::handleBigtableDeleteTable` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances/{instance}/tables/{table}/authorizedViews` | ✓ `simulators/gcp/bigtable.go:142::handleBigtableCreateAuthView` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/tables/{table}/authorizedViews` | ✓ `simulators/gcp/bigtable.go:143::handleBigtableListAuthViews` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances/{instance}/tables/{table}/authorizedViews/{authViewAction}` | ✓ `simulators/gcp/bigtable.go:144::handleBigtableAuthViewItemAction` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/tables/{table}/authorizedViews/{authView}` | ✓ `simulators/gcp/bigtable.go:145::handleBigtableGetAuthView` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v2/projects/{project}/instances/{instance}/tables/{table}/authorizedViews/{authView}` | ✓ `simulators/gcp/bigtable.go:146::handleBigtablePatchAuthView` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/projects/{project}/instances/{instance}/tables/{table}/authorizedViews/{authView}` | ✓ `simulators/gcp/bigtable.go:147::handleBigtableDeleteAuthView` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances/{instance}/tables/{table}/schemaBundles` | ✓ `simulators/gcp/bigtable.go:150::handleBigtableCreateSchemaBundle` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/tables/{table}/schemaBundles` | ✓ `simulators/gcp/bigtable.go:151::handleBigtableListSchemaBundles` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances/{instance}/tables/{table}/schemaBundles/{schemaBundleAction}` | ✓ `simulators/gcp/bigtable.go:152::handleBigtableSchemaBundleItemAction` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/tables/{table}/schemaBundles/{schemaBundle}` | ✓ `simulators/gcp/bigtable.go:153::handleBigtableGetSchemaBundle` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v2/projects/{project}/instances/{instance}/tables/{table}/schemaBundles/{schemaBundle}` | ✓ `simulators/gcp/bigtable.go:154::handleBigtablePatchSchemaBundle` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/projects/{project}/instances/{instance}/tables/{table}/schemaBundles/{schemaBundle}` | ✓ `simulators/gcp/bigtable.go:155::handleBigtableDeleteSchemaBundle` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances/{instance}/logicalViews` | ✓ `simulators/gcp/bigtable.go:158::handleBigtableCreateLogicalView` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/logicalViews` | ✓ `simulators/gcp/bigtable.go:159::handleBigtableListLogicalViews` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances/{instance}/logicalViews/{logicalViewAction}` | ✓ `simulators/gcp/bigtable.go:160::handleBigtableLogicalViewItemAction` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/logicalViews/{logicalView}` | ✓ `simulators/gcp/bigtable.go:161::handleBigtableGetLogicalView` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v2/projects/{project}/instances/{instance}/logicalViews/{logicalView}` | ✓ `simulators/gcp/bigtable.go:162::handleBigtablePatchLogicalView` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/projects/{project}/instances/{instance}/logicalViews/{logicalView}` | ✓ `simulators/gcp/bigtable.go:163::handleBigtableDeleteLogicalView` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances/{instance}/materializedViews` | ✓ `simulators/gcp/bigtable.go:166::handleBigtableCreateMatView` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/materializedViews` | ✓ `simulators/gcp/bigtable.go:167::handleBigtableListMatViews` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/instances/{instance}/materializedViews/{matViewAction}` | ✓ `simulators/gcp/bigtable.go:168::handleBigtableMatViewItemAction` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/instances/{instance}/materializedViews/{matView}` | ✓ `simulators/gcp/bigtable.go:169::handleBigtableGetMatView` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v2/projects/{project}/instances/{instance}/materializedViews/{matView}` | ✓ `simulators/gcp/bigtable.go:170::handleBigtablePatchMatView` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/projects/{project}/instances/{instance}/materializedViews/{matView}` | ✓ `simulators/gcp/bigtable.go:171::handleBigtableDeleteMatView` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
