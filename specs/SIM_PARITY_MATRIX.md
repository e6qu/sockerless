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

Backends: ECS (Fargate), Lambda. Sim: `simulators/aws/`. **Audit completed 2026-04-26.** 33/33 calls implemented after BUG-832 fix.

| Service | Method | Used by | Sim status | Notes |
|---|---|---|---|---|
| ECS | DescribeClusters | ECS | ✓ | `handleECSDescribeClusters` (ecs.go:281) |
| ECS | DescribeTasks | ECS | ✓ | `handleECSDescribeTasks` (ecs.go:833) |
| ECS | ListTasks | ECS | ✓ | `handleECSListTasks` (ecs.go:930) |
| ECS | RunTask | ECS | ✓ | `handleECSRunTask` (ecs.go:473) — full task-def + VPC config |
| ECS | StopTask | ECS | ✓ | `handleECSStopTask` (ecs.go:873) |
| ECS | RegisterTaskDefinition | ECS | ✓ | revision tracking |
| ECS | DeregisterTaskDefinition | ECS | ✓ | |
| ECS | TagResource | ECS | ✓ | **BUG-832 (2026-04-26)** — sim was missing this handler; backend's BUG-781 kill-signal tag + BUG-772 restart-count tag + rename tag silently no-op'd against the sim. Added `handleECSTagResource` + `handleECSUntagResource` in `simulators/aws/ecs.go` plus `mergeECSTagsByKey` helper; rejects STOPPED tasks like real ECS. |
| ECS | ListTagsForResource | ECS | ✓ | `handleECSListTagsForResource` (ecs.go:1020) |
| ECS | ExecuteCommand | ECS | ✓ | BUG-789/798 fix verified live; sim parity via WebSocket + SSM frame writer |
| Lambda | CreateFunction | Lambda | ✓ | tags + VpcConfig + ImageConfig |
| Lambda | DeleteFunction | Lambda | ✓ | |
| Lambda | Invoke | Lambda | ✓ | container-based exec via Runtime API sidecar (BUG-744) |
| Lambda | UpdateFunctionConfiguration | Lambda | ✓ | |
| Lambda | TagResource | Lambda | ✓ | BUG-811 persisted InvocationResult to tags. |
| Lambda | ListTags | Lambda | ✓ | |
| ECR | CreatePullThroughCacheRule | ECS, Lambda | ✓ | |
| ECR | DescribePullThroughCacheRules | ECS, Lambda | ✓ | filters by prefix |
| ECR | CreateRepository | aws-common | ✓ | |
| ECR | BatchDeleteImage | aws-common | ✓ | BUG-825 surfaces real errors now. |
| ECR | GetAuthorizationToken | aws-common | ✓ | |
| CloudWatch Logs | DescribeLogStreams | ECS, Lambda | ✓ | |
| CloudWatch Logs | GetLogEvents | ECS, Lambda | ✓ | pagination |
| EFS | DescribeFileSystems / CreateFileSystem / CreateMountTarget / DescribeMountTargets / CreateAccessPoint | aws-common | ✓ | All five EFS calls under `EnsureFilesystem` helper |
| ServiceDiscovery (Cloud Map) | CreatePrivateDnsNamespace | ECS | ✓ | also creates Docker network for sim cross-talk |
| ServiceDiscovery | DeleteNamespace | ECS | ✓ | |
| ServiceDiscovery | GetNamespace | ECS | ✓ | |
| ServiceDiscovery | ListNamespaces | ECS | ✓ | |
| ServiceDiscovery | CreateService | ECS | ✓ | |
| ServiceDiscovery | DeleteService | ECS | ✓ | |
| ServiceDiscovery | ListServices | ECS | ✓ | filters by namespace |
| ServiceDiscovery | RegisterInstance | ECS | ✓ | |
| ServiceDiscovery | DeregisterInstance | ECS | ✓ | |
| ServiceDiscovery | ListInstances | ECS | ✓ | |
| ServiceDiscovery | DiscoverInstances | ECS | ✓ | DNS discovery |
| ServiceDiscovery | ListTagsForResource | ECS | ✓ | |
| ServiceDiscovery | GetOperation | ECS | ✓ | |

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
