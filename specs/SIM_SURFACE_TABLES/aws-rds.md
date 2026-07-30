# Sim surface — aws-rds

Surface registered in `simulators/aws/rds.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `Action CreateDBInstance` | ✓ `simulators/aws/rds.go:276::handleRDSCreate` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeDBInstances` | ✓ `simulators/aws/rds.go:277::handleRDSDescribe` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ModifyDBInstance` | ✓ `simulators/aws/rds.go:278::handleRDSModify` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteDBInstance` | ✓ `simulators/aws/rds.go:279::handleRDSDelete` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AddTagsToResource` | ✓ `simulators/aws/rds.go:280::handleRDSAddTags` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ListTagsForResource` | ✓ `simulators/aws/rds.go:281::handleRDSListTags` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action RemoveTagsFromResource` | ✓ `simulators/aws/rds.go:282::handleRDSRemoveTags` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateDBSnapshot` | ✓ `simulators/aws/rds.go:283::handleRDSCreateSnapshot` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeDBSnapshots` | ✓ `simulators/aws/rds.go:284::handleRDSDescribeSnapshots` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeDBSnapshotAttributes` | ✓ `simulators/aws/rds.go:285::handleRDSDescribeSnapshotAttributes` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteDBSnapshot` | ✓ `simulators/aws/rds.go:286::handleRDSDeleteSnapshot` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action RestoreDBInstanceFromDBSnapshot` | ✓ `simulators/aws/rds.go:287::handleRDSRestoreFromSnapshot` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CopyDBSnapshot` | ✓ `simulators/aws/rds.go:288::handleRDSCopySnapshot` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action RebootDBInstance` | ✓ `simulators/aws/rds.go:289::handleRDSReboot` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateDBInstanceReadReplica` | ✓ `simulators/aws/rds.go:290::handleRDSCreateReadReplica` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action StartDBInstance` | ✓ `simulators/aws/rds.go:291::handleRDSStartInstance` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action StopDBInstance` | ✓ `simulators/aws/rds.go:292::handleRDSStopInstance` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action PromoteReadReplica` | ✓ `simulators/aws/rds.go:293::handleRDSPromoteReadReplica` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ModifyDBSnapshotAttribute` | ✓ `simulators/aws/rds.go:294::handleRDSModifySnapshotAttribute` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeDBParameters` | ✓ `simulators/aws/rds.go:295::handleRDSDescribeParameters` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ModifyDBParameterGroup` | ✓ `simulators/aws/rds.go:296::handleRDSModifyParameterGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ResetDBParameterGroup` | ✓ `simulators/aws/rds.go:297::handleRDSResetParameterGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateDBCluster` | ✓ `simulators/aws/rds.go:300::handleRDSCreateCluster` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeDBClusters` | ✓ `simulators/aws/rds.go:301::handleRDSDescribeClusters` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ModifyDBCluster` | ✓ `simulators/aws/rds.go:302::handleRDSModifyCluster` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteDBCluster` | ✓ `simulators/aws/rds.go:303::handleRDSDeleteCluster` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action StartDBCluster` | ✓ `simulators/aws/rds.go:304::handleRDSStartCluster` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action StopDBCluster` | ✓ `simulators/aws/rds.go:305::handleRDSStopCluster` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action FailoverDBCluster` | ✓ `simulators/aws/rds.go:306::handleRDSFailoverCluster` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeDBClusterParameters` | ✓ `simulators/aws/rds.go:307::handleRDSDescribeClusterParameters` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ModifyDBClusterParameterGroup` | ✓ `simulators/aws/rds.go:308::handleRDSModifyClusterParameterGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateGlobalCluster` | ✓ `simulators/aws/rds.go:311::handleRDSCreateGlobalCluster` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeGlobalClusters` | ✓ `simulators/aws/rds.go:312::handleRDSDescribeGlobalClusters` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ModifyGlobalCluster` | ✓ `simulators/aws/rds.go:313::handleRDSModifyGlobalCluster` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteGlobalCluster` | ✓ `simulators/aws/rds.go:314::handleRDSDeleteGlobalCluster` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateEventSubscription` | ✓ `simulators/aws/rds.go:317::handleRDSCreateEventSubscription` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeEventSubscriptions` | ✓ `simulators/aws/rds.go:318::handleRDSDescribeEventSubscriptions` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ModifyEventSubscription` | ✓ `simulators/aws/rds.go:319::handleRDSModifyEventSubscription` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteEventSubscription` | ✓ `simulators/aws/rds.go:320::handleRDSDeleteEventSubscription` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateDBClusterEndpoint` | ✓ `simulators/aws/rds.go:323::handleRDSCreateClusterEndpoint` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeDBClusterEndpoints` | ✓ `simulators/aws/rds.go:324::handleRDSDescribeClusterEndpoints` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteDBClusterEndpoint` | ✓ `simulators/aws/rds.go:325::handleRDSDeleteClusterEndpoint` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateDBClusterSnapshot` | ✓ `simulators/aws/rds.go:328::handleRDSCreateClusterSnapshot` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeDBClusterSnapshots` | ✓ `simulators/aws/rds.go:329::handleRDSDescribeClusterSnapshots` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteDBClusterSnapshot` | ✓ `simulators/aws/rds.go:330::handleRDSDeleteClusterSnapshot` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CopyDBClusterSnapshot` | ✓ `simulators/aws/rds.go:331::handleRDSCopyClusterSnapshot` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateDBClusterParameterGroup` | ✓ `simulators/aws/rds.go:334::handleRDSCreateClusterParamGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeDBClusterParameterGroups` | ✓ `simulators/aws/rds.go:335::handleRDSDescribeClusterParamGroups` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteDBClusterParameterGroup` | ✓ `simulators/aws/rds.go:336::handleRDSDeleteClusterParamGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateOptionGroup` | ✓ `simulators/aws/rds.go:339::handleRDSCreateOptionGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeOptionGroups` | ✓ `simulators/aws/rds.go:340::handleRDSDescribeOptionGroups` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteOptionGroup` | ✓ `simulators/aws/rds.go:341::handleRDSDeleteOptionGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeEvents` | ✓ `simulators/aws/rds.go:343::handleRDSDescribeEvents` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeEventCategories` | ✓ `simulators/aws/rds.go:344::handleRDSDescribeEventCategories` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeDBEngineVersions` | ✓ `simulators/aws/rds.go:345::handleRDSDescribeEngineVersions` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeOrderableDBInstanceOptions` | ✓ `simulators/aws/rds.go:346::handleRDSDescribeOrderableOptions` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateDBSubnetGroup` | ✓ `simulators/aws/rds.go:349::handleRDSCreateSubnetGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeDBSubnetGroups` | ✓ `simulators/aws/rds.go:350::handleRDSDescribeSubnetGroups` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ModifyDBSubnetGroup` | ✓ `simulators/aws/rds.go:351::handleRDSModifySubnetGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteDBSubnetGroup` | ✓ `simulators/aws/rds.go:352::handleRDSDeleteSubnetGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateDBParameterGroup` | ✓ `simulators/aws/rds.go:355::handleRDSCreateParamGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeDBParameterGroups` | ✓ `simulators/aws/rds.go:356::handleRDSDescribeParamGroups` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteDBParameterGroup` | ✓ `simulators/aws/rds.go:357::handleRDSDeleteParamGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateCustomDBEngineVersion` | ✓ `simulators/aws/rds_complete.go:100::handleRDSCreateCustomEngineVersion` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ModifyCustomDBEngineVersion` | ✓ `simulators/aws/rds_complete.go:101::handleRDSModifyCustomEngineVersion` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteCustomDBEngineVersion` | ✓ `simulators/aws/rds_complete.go:102::handleRDSDeleteCustomEngineVersion` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeDBRecommendations` | ✓ `simulators/aws/rds_complete.go:104::handleRDSDescribeRecommendations` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ModifyDBRecommendation` | ✓ `simulators/aws/rds_complete.go:105::handleRDSModifyRecommendation` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeDBSnapshotTenantDatabases` | ✓ `simulators/aws/rds_complete.go:107::handleRDSDescribeSnapshotTenantDatabases` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeServerlessV2PlatformVersions` | ✓ `simulators/aws/rds_complete.go:108::handleRDSDescribeServerlessV2PlatformVersions` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeValidDBInstanceModifications` | ✓ `simulators/aws/rds_complete.go:109::handleRDSDescribeValidDBInstanceModifications` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ModifyCurrentDBClusterCapacity` | ✓ `simulators/aws/rds_complete.go:111::handleRDSModifyCurrentDBClusterCapacity` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ModifyOptionGroup` | ✓ `simulators/aws/rds_complete.go:112::handleRDSModifyOptionGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action StartDBInstanceAutomatedBackupsReplication` | ✓ `simulators/aws/rds_complete.go:114::handleRDSStartAutomatedBackupsReplication` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action StopDBInstanceAutomatedBackupsReplication` | ✓ `simulators/aws/rds_complete.go:115::handleRDSStopAutomatedBackupsReplication` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action SwitchoverGlobalCluster` | ✓ `simulators/aws/rds_complete.go:117::handleRDSSwitchoverGlobalCluster` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action SwitchoverReadReplica` | ✓ `simulators/aws/rds_complete.go:118::handleRDSSwitchoverReadReplica` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeAccountAttributes` | ✓ `simulators/aws/rds_complete.go:120::handleRDSDescribeAccountAttributes` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateDBProxy` | ✓ `simulators/aws/rds_proxies_roles.go:216::handleRDSCreateProxy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeDBProxies` | ✓ `simulators/aws/rds_proxies_roles.go:217::handleRDSDescribeProxies` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ModifyDBProxy` | ✓ `simulators/aws/rds_proxies_roles.go:218::handleRDSModifyProxy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteDBProxy` | ✓ `simulators/aws/rds_proxies_roles.go:219::handleRDSDeleteProxy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateDBProxyEndpoint` | ✓ `simulators/aws/rds_proxies_roles.go:220::handleRDSCreateProxyEndpoint` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeDBProxyEndpoints` | ✓ `simulators/aws/rds_proxies_roles.go:221::handleRDSDescribeProxyEndpoints` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ModifyDBProxyEndpoint` | ✓ `simulators/aws/rds_proxies_roles.go:222::handleRDSModifyProxyEndpoint` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteDBProxyEndpoint` | ✓ `simulators/aws/rds_proxies_roles.go:223::handleRDSDeleteProxyEndpoint` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeDBProxyTargets` | ✓ `simulators/aws/rds_proxies_roles.go:224::handleRDSDescribeProxyTargets` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeDBProxyTargetGroups` | ✓ `simulators/aws/rds_proxies_roles.go:225::handleRDSDescribeProxyTargetGroups` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ModifyDBProxyTargetGroup` | ✓ `simulators/aws/rds_proxies_roles.go:226::handleRDSModifyProxyTargetGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action RegisterDBProxyTargets` | ✓ `simulators/aws/rds_proxies_roles.go:227::handleRDSRegisterProxyTargets` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeregisterDBProxyTargets` | ✓ `simulators/aws/rds_proxies_roles.go:228::handleRDSDeregisterProxyTargets` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AddRoleToDBCluster` | ✓ `simulators/aws/rds_proxies_roles.go:231::handleRDSAddRoleToCluster` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action RemoveRoleFromDBCluster` | ✓ `simulators/aws/rds_proxies_roles.go:232::handleRDSRemoveRoleFromCluster` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AddRoleToDBInstance` | ✓ `simulators/aws/rds_proxies_roles.go:233::handleRDSAddRoleToInstance` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action RemoveRoleFromDBInstance` | ✓ `simulators/aws/rds_proxies_roles.go:234::handleRDSRemoveRoleFromInstance` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateDBSecurityGroup` | ✓ `simulators/aws/rds_proxies_roles.go:237::handleRDSCreateSecurityGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeDBSecurityGroups` | ✓ `simulators/aws/rds_proxies_roles.go:238::handleRDSDescribeSecurityGroups` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteDBSecurityGroup` | ✓ `simulators/aws/rds_proxies_roles.go:239::handleRDSDeleteSecurityGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AuthorizeDBSecurityGroupIngress` | ✓ `simulators/aws/rds_proxies_roles.go:240::handleRDSAuthorizeSecurityGroupIngress` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action RevokeDBSecurityGroupIngress` | ✓ `simulators/aws/rds_proxies_roles.go:241::handleRDSRevokeSecurityGroupIngress` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeCertificates` | ✓ `simulators/aws/rds_proxies_roles.go:244::handleRDSDescribeCertificates` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ModifyCertificates` | ✓ `simulators/aws/rds_proxies_roles.go:245::handleRDSModifyCertificates` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeDBInstanceAutomatedBackups` | ✓ `simulators/aws/rds_proxies_roles.go:248::handleRDSDescribeInstanceAutomatedBackups` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteDBInstanceAutomatedBackup` | ✓ `simulators/aws/rds_proxies_roles.go:249::handleRDSDeleteInstanceAutomatedBackup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeDBClusterAutomatedBackups` | ✓ `simulators/aws/rds_proxies_roles.go:250::handleRDSDescribeClusterAutomatedBackups` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteDBClusterAutomatedBackup` | ✓ `simulators/aws/rds_proxies_roles.go:251::handleRDSDeleteClusterAutomatedBackup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeDBLogFiles` | ✓ `simulators/aws/rds_proxies_roles.go:254::handleRDSDescribeLogFiles` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DownloadDBLogFilePortion` | ✓ `simulators/aws/rds_proxies_roles.go:255::handleRDSDownloadLogFilePortion` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CopyDBClusterParameterGroup` | ✓ `simulators/aws/rds_proxies_roles.go:258::handleRDSCopyClusterParameterGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CopyDBParameterGroup` | ✓ `simulators/aws/rds_proxies_roles.go:259::handleRDSCopyParameterGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CopyOptionGroup` | ✓ `simulators/aws/rds_proxies_roles.go:260::handleRDSCopyOptionGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AddSourceIdentifierToSubscription` | ✓ `simulators/aws/rds_proxies_roles.go:263::handleRDSAddSourceIdentifier` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action RemoveSourceIdentifierFromSubscription` | ✓ `simulators/aws/rds_proxies_roles.go:264::handleRDSRemoveSourceIdentifier` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ApplyPendingMaintenanceAction` | ✓ `simulators/aws/rds_proxies_roles.go:267::handleRDSApplyPendingMaintenanceAction` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribePendingMaintenanceActions` | ✓ `simulators/aws/rds_proxies_roles.go:268::handleRDSDescribePendingMaintenanceActions` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action RestoreDBClusterFromSnapshot` | ✓ `simulators/aws/rds_restore_extras.go:38::handleRDSRestoreClusterFromSnapshot` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action RestoreDBClusterToPointInTime` | ✓ `simulators/aws/rds_restore_extras.go:39::handleRDSRestoreClusterToPointInTime` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action RestoreDBInstanceToPointInTime` | ✓ `simulators/aws/rds_restore_extras.go:40::handleRDSRestoreInstanceToPointInTime` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action RestoreDBClusterFromS3` | ✓ `simulators/aws/rds_restore_extras.go:41::handleRDSRestoreClusterFromS3` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action RestoreDBInstanceFromS3` | ✓ `simulators/aws/rds_restore_extras.go:42::handleRDSRestoreInstanceFromS3` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeReservedDBInstances` | ✓ `simulators/aws/rds_restore_extras.go:45::handleRDSDescribeReservedInstances` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeReservedDBInstancesOfferings` | ✓ `simulators/aws/rds_restore_extras.go:46::handleRDSDescribeReservedInstancesOfferings` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action PurchaseReservedDBInstancesOffering` | ✓ `simulators/aws/rds_restore_extras.go:47::handleRDSPurchaseReservedInstancesOffering` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateBlueGreenDeployment` | ✓ `simulators/aws/rds_restore_extras.go:50::handleRDSCreateBlueGreenDeployment` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeBlueGreenDeployments` | ✓ `simulators/aws/rds_restore_extras.go:51::handleRDSDescribeBlueGreenDeployments` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteBlueGreenDeployment` | ✓ `simulators/aws/rds_restore_extras.go:52::handleRDSDeleteBlueGreenDeployment` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action SwitchoverBlueGreenDeployment` | ✓ `simulators/aws/rds_restore_extras.go:53::handleRDSSwitchoverBlueGreenDeployment` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateIntegration` | ✓ `simulators/aws/rds_restore_extras.go:56::handleRDSCreateIntegration` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeIntegrations` | ✓ `simulators/aws/rds_restore_extras.go:57::handleRDSDescribeIntegrations` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ModifyIntegration` | ✓ `simulators/aws/rds_restore_extras.go:58::handleRDSModifyIntegration` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteIntegration` | ✓ `simulators/aws/rds_restore_extras.go:59::handleRDSDeleteIntegration` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateTenantDatabase` | ✓ `simulators/aws/rds_restore_extras.go:62::handleRDSCreateTenantDatabase` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeTenantDatabases` | ✓ `simulators/aws/rds_restore_extras.go:63::handleRDSDescribeTenantDatabases` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ModifyTenantDatabase` | ✓ `simulators/aws/rds_restore_extras.go:64::handleRDSModifyTenantDatabase` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteTenantDatabase` | ✓ `simulators/aws/rds_restore_extras.go:65::handleRDSDeleteTenantDatabase` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateDBShardGroup` | ✓ `simulators/aws/rds_restore_extras.go:68::handleRDSCreateShardGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeDBShardGroups` | ✓ `simulators/aws/rds_restore_extras.go:69::handleRDSDescribeShardGroups` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ModifyDBShardGroup` | ✓ `simulators/aws/rds_restore_extras.go:70::handleRDSModifyShardGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteDBShardGroup` | ✓ `simulators/aws/rds_restore_extras.go:71::handleRDSDeleteShardGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action RebootDBShardGroup` | ✓ `simulators/aws/rds_restore_extras.go:72::handleRDSRebootShardGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action StartActivityStream` | ✓ `simulators/aws/rds_restore_extras.go:75::handleRDSStartActivityStream` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action StopActivityStream` | ✓ `simulators/aws/rds_restore_extras.go:76::handleRDSStopActivityStream` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ModifyActivityStream` | ✓ `simulators/aws/rds_restore_extras.go:77::handleRDSModifyActivityStream` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action BacktrackDBCluster` | ✓ `simulators/aws/rds_restore_extras.go:80::handleRDSBacktrackCluster` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeDBClusterBacktracks` | ✓ `simulators/aws/rds_restore_extras.go:81::handleRDSDescribeClusterBacktracks` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action StartExportTask` | ✓ `simulators/aws/rds_restore_extras.go:84::handleRDSStartExportTask` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeExportTasks` | ✓ `simulators/aws/rds_restore_extras.go:85::handleRDSDescribeExportTasks` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CancelExportTask` | ✓ `simulators/aws/rds_restore_extras.go:86::handleRDSCancelExportTask` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action RebootDBCluster` | ✓ `simulators/aws/rds_restore_extras.go:89::handleRDSRebootCluster` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ResetDBClusterParameterGroup` | ✓ `simulators/aws/rds_restore_extras.go:90::handleRDSResetClusterParameterGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ModifyDBClusterEndpoint` | ✓ `simulators/aws/rds_restore_extras.go:91::handleRDSModifyClusterEndpoint` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action FailoverGlobalCluster` | ✓ `simulators/aws/rds_restore_extras.go:92::handleRDSFailoverGlobalCluster` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action RemoveFromGlobalCluster` | ✓ `simulators/aws/rds_restore_extras.go:93::handleRDSRemoveFromGlobalCluster` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action PromoteReadReplicaDBCluster` | ✓ `simulators/aws/rds_restore_extras.go:94::handleRDSPromoteReadReplicaCluster` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action EnableHttpEndpoint` | ✓ `simulators/aws/rds_restore_extras.go:95::handleRDSEnableHTTPEndpoint` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DisableHttpEndpoint` | ✓ `simulators/aws/rds_restore_extras.go:96::handleRDSDisableHTTPEndpoint` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ModifyDBClusterSnapshotAttribute` | ✓ `simulators/aws/rds_restore_extras.go:99::handleRDSModifyClusterSnapshotAttribute` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeDBClusterSnapshotAttributes` | ✓ `simulators/aws/rds_restore_extras.go:100::handleRDSDescribeClusterSnapshotAttributes` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ModifyDBSnapshot` | ✓ `simulators/aws/rds_restore_extras.go:101::handleRDSModifySnapshot` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeOptionGroupOptions` | ✓ `simulators/aws/rds_restore_extras.go:104::handleRDSDescribeOptionGroupOptions` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeEngineDefaultParameters` | ✓ `simulators/aws/rds_restore_extras.go:105::handleRDSDescribeEngineDefaultParameters` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeEngineDefaultClusterParameters` | ✓ `simulators/aws/rds_restore_extras.go:106::handleRDSDescribeEngineDefaultClusterParameters` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeSourceRegions` | ✓ `simulators/aws/rds_restore_extras.go:107::handleRDSDescribeSourceRegions` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeDBMajorEngineVersions` | ✓ `simulators/aws/rds_restore_extras.go:108::handleRDSDescribeDBMajorEngineVersions` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
Amazon Relational Database Service DB instances using PostgreSQL, MySQL, or
MariaDB expose the native database protocol at the returned `Endpoint`. The
engine starts lazily in its real vendor container, retains data in an
instance-owned volume, terminates TLS at the service endpoint, and accepts
either the encrypted master credential or a TLS-protected, 15-minute SigV4 IAM
database authentication token authorized through `rds-db:connect`.
`ModifyDBInstance` changes IAM authentication and rotates the actual database
account both while running and across a stopped/start lifecycle without
replacing the volume. Stock pgx and MySQL drivers prove authentication denial,
TLS enforcement, password rotation, stop/start persistence, and SQL reads and
writes against all three engines.
<!-- HAND-WRITTEN END -->
