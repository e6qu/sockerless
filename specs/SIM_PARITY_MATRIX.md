# Cross-simulator feature parity matrix (Phase 108)

This file is the source-of-truth tracker for **Phase 108 — cross-simulator feature parity audit** (queued in [PLAN.md](../PLAN.md)).

Each row is a cloud-API call sockerless makes from one of the 7 backends. Each column is one of the three simulators. The cell records whether the simulator implements the call at the fidelity sockerless requires.

Legend:
- ✓ — sim implements the call at sockerless's required fidelity (used by integration tests; passes against the sim).
- ⚠ — sim implements the call but with reduced fidelity (e.g. always returns the same fake field; missing pagination; ignores filters). A bug to close in Phase 108.
- ✗ — sim does not implement the call. A bug to close in Phase 108.
- — — call is not made by any backend pointed at this cloud (not applicable).

The matrix was seeded from a `grep -roh '\b[ps]\.<provider>\.<Service>\.<Method>\b'` sweep across every backend on `2026-04-26`. Entries are filled in as Phase 108 audits each row against the corresponding sim handler in `simulators/<provider>/`. **Per the project no-defer rule, every ⚠ / ✗ row is a BUG that gets a real fix in this phase.**

## AWS

Backends: ECS (Fargate), Lambda. Sim: `simulators/aws/`.

| Service | Method | Used by | Sim status | Notes |
|---|---|---|---|---|
| ECS | DescribeClusters | ECS | tbd | |
| ECS | DescribeTasks | ECS | tbd | |
| ECS | ListTasks | ECS | tbd | |
| ECS | RunTask | ECS | tbd | |
| ECS | StopTask | ECS | tbd | |
| ECS | RegisterTaskDefinition | ECS | tbd | |
| ECS | DeregisterTaskDefinition | ECS | tbd | |
| ECS | TagResource | ECS | tbd | |
| ECS | ListTagsForResource | ECS | tbd | |
| ECS | ExecuteCommand | ECS | tbd | BUG-789/798 fix verified live; sim parity verified post-fix. |
| Lambda | CreateFunction | Lambda | tbd | |
| Lambda | DeleteFunction | Lambda | tbd | |
| Lambda | Invoke | Lambda | tbd | |
| Lambda | UpdateFunctionConfiguration | Lambda | tbd | |
| Lambda | TagResource | Lambda | tbd | BUG-811 persisted InvocationResult to tags. |
| Lambda | ListTags | Lambda | tbd | |
| ECR | CreatePullThroughCacheRule | ECS, Lambda | tbd | |
| ECR | DescribePullThroughCacheRules | ECS, Lambda | tbd | |
| ECR | CreateRepository | aws-common | tbd | |
| ECR | BatchDeleteImage | aws-common | tbd | BUG-825 surfaces real errors now. |
| ECR | GetAuthorizationToken | aws-common | tbd | |
| CloudWatch Logs | DescribeLogStreams | ECS, Lambda | tbd | |
| CloudWatch Logs | GetLogEvents | ECS, Lambda | tbd | |
| EFS | EnsureFilesystem (helper) | aws-common | tbd | |
| ServiceDiscovery (Cloud Map) | CreatePrivateDnsNamespace | ECS | tbd | |
| ServiceDiscovery | DeleteNamespace | ECS | tbd | |
| ServiceDiscovery | GetNamespace | ECS | tbd | |
| ServiceDiscovery | ListNamespaces | ECS | tbd | |
| ServiceDiscovery | CreateService | ECS | tbd | |
| ServiceDiscovery | DeleteService | ECS | tbd | |
| ServiceDiscovery | ListServices | ECS | tbd | |
| ServiceDiscovery | RegisterInstance | ECS | tbd | |
| ServiceDiscovery | DeregisterInstance | ECS | tbd | |
| ServiceDiscovery | ListInstances | ECS | tbd | |
| ServiceDiscovery | DiscoverInstances | ECS | tbd | |
| ServiceDiscovery | ListTagsForResource | ECS | tbd | |
| ServiceDiscovery | GetOperation | ECS | tbd | |

## GCP

Backends: Cloud Run Jobs (cloudrun), Cloud Run Functions (cloudrun-functions). Sim: `simulators/gcp/`.

| Service | Method | Used by | Sim status | Notes |
|---|---|---|---|---|
| Cloud Run | Jobs.CreateJob | cloudrun | tbd | |
| Cloud Run | Jobs.DeleteJob | cloudrun | tbd | |
| Cloud Run | Jobs.ListJobs | cloudrun | tbd | |
| Cloud Run | Jobs.RunJob | cloudrun | tbd | |
| Cloud Run | Executions.GetExecution | cloudrun | tbd | |
| Cloud Run | Executions.CancelExecution | cloudrun | tbd | |
| Cloud Run | Services.CreateService | cloudrun | tbd | |
| Cloud Run | Services.GetService | cloudrun | tbd | |
| Cloud Run | Services.UpdateService | cloudrun | tbd | |
| Cloud Run | Services.DeleteService | cloudrun | tbd | |
| Cloud Functions | CreateFunction | cloudrun-functions | tbd | |
| Cloud Functions | DeleteFunction | cloudrun-functions | tbd | |
| Cloud Functions | ListFunctions | cloudrun-functions | tbd | |
| Cloud Logging | LogAdmin.Entries | cloudrun, cloudrun-functions | tbd | |
| Cloud DNS | ManagedZones | cloudrun, cloudrun-functions | tbd | |
| Cloud DNS | ResourceRecordSets | cloudrun, cloudrun-functions | tbd | |

## Azure

Backends: Container Apps (aca), Azure Functions (azure-functions). Sim: `simulators/azure/`.

| Service | Method | Used by | Sim status | Notes |
|---|---|---|---|---|
| Container Apps | Jobs.BeginCreateOrUpdate | aca | tbd | |
| Container Apps | Jobs.BeginDelete | aca | tbd | |
| Container Apps | Jobs.BeginStart | aca | tbd | |
| Container Apps | Jobs.BeginStopExecution | aca | tbd | |
| Container Apps | Jobs.NewListByResourceGroupPager | aca | tbd | |
| Container Apps | ContainerApps.BeginCreateOrUpdate | aca (UseApp) | tbd | |
| Container Apps | ContainerApps.BeginDelete | aca (UseApp) | tbd | |
| Container Apps | ContainerApps.Get | aca (UseApp) | tbd | |
| Container Apps | EnvStorages.CreateOrUpdate | aca | tbd | |
| Container Apps | EnvStorages.Delete | aca | tbd | |
| Container Apps | Executions.NewListPager | aca | tbd | |
| Network | NSG.BeginCreateOrUpdate | aca | tbd | |
| Network | NSG.BeginDelete | aca | tbd | |
| Network | NSG.Get | aca | tbd | |
| Network | NSGRules.BeginCreateOrUpdate | aca | tbd | |
| Private DNS | PrivateDNSZones.BeginCreateOrUpdate | aca | tbd | |
| Private DNS | PrivateDNSZones.BeginDelete | aca | tbd | |
| Private DNS | PrivateDNSZones.Get | aca | tbd | |
| Private DNS | PrivateDNSRecords.CreateOrUpdate | aca | tbd | |
| Private DNS | PrivateDNSRecords.Delete | aca | tbd | |
| Private DNS | PrivateDNSRecords.Get | aca | tbd | |
| Storage | StorageAccounts.ListKeys | aca, azure-functions | tbd | |
| Log Analytics | Logs.QueryWorkspace | aca, azure-functions | tbd | |
| Log Analytics | LogsHTTP.QueryWorkspace | aca | tbd | (HTTP fallback path for non-TLS sim runs) |
| App Service | WebApps.BeginCreateOrUpdate | azure-functions | tbd | |
| App Service | WebApps.Delete | azure-functions | tbd | |
| App Service | WebApps.NewListByResourceGroupPager | azure-functions | tbd | |
| App Service | WebApps.UpdateAzureStorageAccounts | azure-functions | tbd | |

## Closure tracking

Each `tbd` cell becomes ✓/⚠/✗ as Phase 108 audits the row. Every ⚠ / ✗ entry is filed as a BUG and fixed in-phase per the no-defer rule.

When all rows show ✓, Phase 108 closes and a follow-up rule is added: any new SDK call added to a backend must update this matrix and add a sim handler in the same commit (the existing "sim parity per commit" rule made stronger).
