# Sim surface — aws-elasticache

Surface registered in `simulators/aws/elasticache_serverless.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `Action CreateServerlessCache` | ✓ `simulators/aws/elasticache_serverless.go:133::handleECCreateServerlessCache` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeServerlessCaches` | ✓ `simulators/aws/elasticache_serverless.go:134::handleECDescribeServerlessCaches` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ModifyServerlessCache` | ✓ `simulators/aws/elasticache_serverless.go:135::handleECModifyServerlessCache` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteServerlessCache` | ✓ `simulators/aws/elasticache_serverless.go:136::handleECDeleteServerlessCache` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateServerlessCacheSnapshot` | ✓ `simulators/aws/elasticache_serverless.go:139::handleECCreateServerlessSnapshot` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeServerlessCacheSnapshots` | ✓ `simulators/aws/elasticache_serverless.go:140::handleECDescribeServerlessSnapshots` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteServerlessCacheSnapshot` | ✓ `simulators/aws/elasticache_serverless.go:141::handleECDeleteServerlessSnapshot` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CopyServerlessCacheSnapshot` | ✓ `simulators/aws/elasticache_serverless.go:142::handleECCopyServerlessSnapshot` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ExportServerlessCacheSnapshot` | ✓ `simulators/aws/elasticache_serverless.go:143::handleECExportServerlessSnapshot` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateGlobalReplicationGroup` | ✓ `simulators/aws/elasticache_serverless.go:146::handleECCreateGlobalReplGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeGlobalReplicationGroups` | ✓ `simulators/aws/elasticache_serverless.go:147::handleECDescribeGlobalReplGroups` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ModifyGlobalReplicationGroup` | ✓ `simulators/aws/elasticache_serverless.go:148::handleECModifyGlobalReplGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteGlobalReplicationGroup` | ✓ `simulators/aws/elasticache_serverless.go:149::handleECDeleteGlobalReplGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DisassociateGlobalReplicationGroup` | ✓ `simulators/aws/elasticache_serverless.go:150::handleECDisassociateGlobalReplGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action FailoverGlobalReplicationGroup` | ✓ `simulators/aws/elasticache_serverless.go:151::handleECFailoverGlobalReplGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action IncreaseNodeGroupsInGlobalReplicationGroup` | ✓ `simulators/aws/elasticache_serverless.go:152::handleECIncreaseNodeGroupsGlobal` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DecreaseNodeGroupsInGlobalReplicationGroup` | ✓ `simulators/aws/elasticache_serverless.go:153::handleECDecreaseNodeGroupsGlobal` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action RebalanceSlotsInGlobalReplicationGroup` | ✓ `simulators/aws/elasticache_serverless.go:154::handleECRebalanceSlotsGlobal` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateCacheSecurityGroup` | ✓ `simulators/aws/elasticache_serverless.go:157::handleECCreateCacheSecGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteCacheSecurityGroup` | ✓ `simulators/aws/elasticache_serverless.go:158::handleECDeleteCacheSecGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AuthorizeCacheSecurityGroupIngress` | ✓ `simulators/aws/elasticache_serverless.go:159::handleECAuthorizeCacheSecGroupIngress` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action RevokeCacheSecurityGroupIngress` | ✓ `simulators/aws/elasticache_serverless.go:160::handleECRevokeCacheSecGroupIngress` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeUpdateActions` | ✓ `simulators/aws/elasticache_serverless.go:163::handleECDescribeUpdateActions` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action BatchApplyUpdateAction` | ✓ `simulators/aws/elasticache_serverless.go:164::handleECBatchApplyUpdateAction` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action BatchStopUpdateAction` | ✓ `simulators/aws/elasticache_serverless.go:165::handleECBatchStopUpdateAction` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action IncreaseReplicaCount` | ✓ `simulators/aws/elasticache_serverless.go:168::handleECIncreaseReplicaCount` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DecreaseReplicaCount` | ✓ `simulators/aws/elasticache_serverless.go:169::handleECDecreaseReplicaCount` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ModifyReplicationGroupShardConfiguration` | ✓ `simulators/aws/elasticache_serverless.go:170::handleECModifyReplGroupShardConfig` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TestFailover` | ✓ `simulators/aws/elasticache_serverless.go:171::handleECTestFailover` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ListAllowedNodeTypeModifications` | ✓ `simulators/aws/elasticache_serverless.go:174::handleECListAllowedNodeTypeModifications` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action PurchaseReservedCacheNodesOffering` | ✓ `simulators/aws/elasticache_serverless.go:175::handleECPurchaseReservedCacheNodesOffering` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action StartMigration` | ✓ `simulators/aws/elasticache_serverless.go:176::handleECStartMigration` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CompleteMigration` | ✓ `simulators/aws/elasticache_serverless.go:177::handleECCompleteMigration` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TestMigration` | ✓ `simulators/aws/elasticache_serverless.go:178::handleECTestMigration` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateCacheCluster` | ✓ `simulators/aws/elasticache.go:144::handleECCreate` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeCacheClusters` | ✓ `simulators/aws/elasticache.go:145::handleECDescribe` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ModifyCacheCluster` | ✓ `simulators/aws/elasticache.go:146::handleECModify` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteCacheCluster` | ✓ `simulators/aws/elasticache.go:147::handleECDelete` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action RebootCacheCluster` | ✓ `simulators/aws/elasticache.go:148::handleECReboot` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateReplicationGroup` | ✓ `simulators/aws/elasticache.go:149::handleECCreateReplGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeReplicationGroups` | ✓ `simulators/aws/elasticache.go:150::handleECDescribeReplGroups` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ModifyReplicationGroup` | ✓ `simulators/aws/elasticache.go:151::handleECModifyReplGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteReplicationGroup` | ✓ `simulators/aws/elasticache.go:152::handleECDeleteReplGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateCacheSubnetGroup` | ✓ `simulators/aws/elasticache.go:153::handleECCreateSubnetGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeCacheSubnetGroups` | ✓ `simulators/aws/elasticache.go:154::handleECDescribeSubnetGroups` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ModifyCacheSubnetGroup` | ✓ `simulators/aws/elasticache.go:155::handleECModifySubnetGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteCacheSubnetGroup` | ✓ `simulators/aws/elasticache.go:156::handleECDeleteSubnetGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateCacheParameterGroup` | ✓ `simulators/aws/elasticache.go:157::handleECCreateParamGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeCacheParameterGroups` | ✓ `simulators/aws/elasticache.go:158::handleECDescribeParamGroups` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteCacheParameterGroup` | ✓ `simulators/aws/elasticache.go:159::handleECDeleteParamGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AddTagsToResource` | ✓ `simulators/aws/elasticache.go:160::handleECAddTags` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ListTagsForResource` | ✓ `simulators/aws/elasticache.go:161::handleECListTags` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action RemoveTagsFromResource` | ✓ `simulators/aws/elasticache.go:162::handleECRemoveTags` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateSnapshot` | ✓ `simulators/aws/elasticache.go:166::handleECCreateSnapshot` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeSnapshots` | ✓ `simulators/aws/elasticache.go:167::handleECDescribeSnapshots` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteSnapshot` | ✓ `simulators/aws/elasticache.go:168::handleECDeleteSnapshot` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CopySnapshot` | ✓ `simulators/aws/elasticache.go:169::handleECCopySnapshot` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateUser` | ✓ `simulators/aws/elasticache.go:170::handleECCreateUser` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeUsers` | ✓ `simulators/aws/elasticache.go:171::handleECDescribeUsers` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ModifyUser` | ✓ `simulators/aws/elasticache.go:172::handleECModifyUser` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteUser` | ✓ `simulators/aws/elasticache.go:173::handleECDeleteUser` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateUserGroup` | ✓ `simulators/aws/elasticache.go:174::handleECCreateUserGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeUserGroups` | ✓ `simulators/aws/elasticache.go:175::handleECDescribeUserGroups` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ModifyUserGroup` | ✓ `simulators/aws/elasticache.go:176::handleECModifyUserGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteUserGroup` | ✓ `simulators/aws/elasticache.go:177::handleECDeleteUserGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeCacheParameters` | ✓ `simulators/aws/elasticache.go:178::handleECDescribeParameters` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ModifyCacheParameterGroup` | ✓ `simulators/aws/elasticache.go:179::handleECModifyParameters` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ResetCacheParameterGroup` | ✓ `simulators/aws/elasticache.go:180::handleECResetParameters` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeEngineDefaultParameters` | ✓ `simulators/aws/elasticache.go:181::handleECDescribeEngineDefaultParameters` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeEvents` | ✓ `simulators/aws/elasticache.go:182::handleECDescribeEvents` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeCacheEngineVersions` | ✓ `simulators/aws/elasticache.go:183::handleECDescribeCacheEngineVersions` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeReservedCacheNodes` | ✓ `simulators/aws/elasticache.go:184::handleECDescribeReservedCacheNodes` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeReservedCacheNodesOfferings` | ✓ `simulators/aws/elasticache.go:185::handleECDescribeReservedCacheNodesOfferings` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeServiceUpdates` | ✓ `simulators/aws/elasticache.go:186::handleECDescribeServiceUpdates` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeCacheSecurityGroups` | ✓ `simulators/aws/elasticache.go:187::handleECDescribeCacheSecurityGroups` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
