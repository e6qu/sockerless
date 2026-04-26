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

Backends: Cloud Run Jobs (cloudrun), Cloud Run Functions (cloudrun-functions). Sim: `simulators/gcp/`. **Audit completed 2026-04-26.** 16/16 calls implemented after BUG-833 fix.

| Service | Method | Used by | Sim status | Notes |
|---|---|---|---|---|
| Cloud Run | Jobs.CreateJob | cloudrun | ✓ | `registerCloudRunJobs` (cloudrunjobs.go:225) — full LRO + job metadata |
| Cloud Run | Jobs.DeleteJob | cloudrun | ✓ | (cloudrunjobs.go:317) — cascades execution delete |
| Cloud Run | Jobs.ListJobs | cloudrun | ✓ | (cloudrunjobs.go:302) — filters by project/location prefix |
| Cloud Run | Jobs.RunJob | cloudrun | ✓ | (cloudrunjobs.go:344) — creates execution with task metadata |
| Cloud Run | Executions.GetExecution | cloudrun | ✓ | (cloudrunjobs.go:539) — full execution state |
| Cloud Run | Executions.CancelExecution | cloudrun | ✓ | (cloudrunjobs.go:571) — stops container + injects cancel log |
| Cloud Run | Services.CreateService | cloudrun (UseService) | ✓ | **BUG-833 (2026-04-26)** — sim only had v1 Knative routes; backend uses run.NewServicesRESTClient (v2 REST). Added `registerCloudRunServicesV2` in `simulators/gcp/cloudrunservices.go` covering Create/Get/List/Update/Delete on `/v2/projects/{p}/locations/{l}/services` with proto-JSON shape (TerminalCondition, LatestReadyRevision, generation as int64-string). |
| Cloud Run | Services.GetService | cloudrun (UseService) | ✓ | (cloudrunservices.go) — service_discovery_cloud.go uses this for CNAME resolution |
| Cloud Run | Services.UpdateService | cloudrun (declarative) | ✓ | (cloudrunservices.go) — terraform `google_cloud_run_v2_service` parity; backend recreates rather than patches today |
| Cloud Run | Services.DeleteService | cloudrun (UseService) | ✓ | (cloudrunservices.go) — LRO + store delete |
| Cloud Functions | CreateFunction | cloudrun-functions | ✓ | (cloudfunctions.go:57) — full LRO + function URI |
| Cloud Functions | DeleteFunction | cloudrun-functions | ✓ | (cloudfunctions.go:181) — LRO |
| Cloud Functions | ListFunctions | cloudrun-functions | ✓ | (cloudfunctions.go:114) — filters by project/location prefix |
| Cloud Logging | LogAdmin.Entries | cloudrun, cloudrun-functions | ✓ | (logging.go:151) — REST ListLogEntries with filter + pageSize |
| Cloud DNS | ManagedZones | cloudrun, cloudrun-functions | ✓ | (dns.go:44/96/114/128) — Create/Get/List/Delete + Docker network backing for private zones |
| Cloud DNS | ResourceRecordSets | cloudrun, cloudrun-functions | ✓ | (dns.go:159/190/236) — List/Create/Delete + Docker network connection for A records |

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
