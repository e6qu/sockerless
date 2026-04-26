# Cross-simulator feature parity matrix

Source of truth for which cloud-API calls each simulator implements at the fidelity sockerless requires. Each row is a cloud-API call sockerless makes from one of the 7 backends. Each column is one of the three simulators.

Legend:
- ✓ — sim implements the call at sockerless's required fidelity (used by integration tests; passes against the sim).
- ⚠ — sim implements the call but with reduced fidelity (e.g. always returns the same fake field; missing pagination; ignores filters). Filed as a bug.
- ✗ — sim does not implement the call. Filed as a bug.
- — — call is not made by any backend pointed at this cloud (not applicable).

**Standing rule:** any new SDK call added to a backend must update this matrix and add the sim handler in the same commit (PLAN.md principle #10). Every ⚠ / ✗ row is a bug that gets a real fix in the same session per the no-defer rule.

## AWS

Backends: ECS (Fargate), Lambda. Sim: `simulators/aws/`. **33/33 ✓.**

| Service | Method | Used by | Sim status | Notes |
|---|---|---|---|---|
| ECS | DescribeClusters | ECS | ✓ | `handleECSDescribeClusters` (ecs.go:281) |
| ECS | DescribeTasks | ECS | ✓ | `handleECSDescribeTasks` (ecs.go:833) |
| ECS | ListTasks | ECS | ✓ | `handleECSListTasks` (ecs.go:930) |
| ECS | RunTask | ECS | ✓ | `handleECSRunTask` (ecs.go:473) — full task-def + VPC config |
| ECS | StopTask | ECS | ✓ | `handleECSStopTask` (ecs.go:873) |
| ECS | RegisterTaskDefinition | ECS | ✓ | revision tracking |
| ECS | DeregisterTaskDefinition | ECS | ✓ | |
| ECS | TagResource | ECS | ✓ | `handleECSTagResource` + `handleECSUntagResource` + `mergeECSTagsByKey` helper; rejects STOPPED / DEPROVISIONING tasks like real ECS. |
| ECS | ListTagsForResource | ECS | ✓ | `handleECSListTagsForResource` (ecs.go:1020) |
| ECS | ExecuteCommand | ECS | ✓ | sim parity via WebSocket + SSM AgentMessage frame writer + exit-code marker |
| Lambda | CreateFunction | Lambda | ✓ | tags + VpcConfig + ImageConfig |
| Lambda | DeleteFunction | Lambda | ✓ | |
| Lambda | Invoke | Lambda | ✓ | container-based exec via Runtime API sidecar |
| Lambda | UpdateFunctionConfiguration | Lambda | ✓ | |
| Lambda | TagResource | Lambda | ✓ | InvocationResult persisted to tags for `docker wait` exit-code recovery. |
| Lambda | ListTags | Lambda | ✓ | |
| ECR | CreatePullThroughCacheRule | ECS, Lambda | ✓ | |
| ECR | DescribePullThroughCacheRules | ECS, Lambda | ✓ | filters by prefix |
| ECR | CreateRepository | aws-common | ✓ | |
| ECR | BatchDeleteImage | aws-common | ✓ | Surfaces real errors via the ImageManager.Remove aggregator. |
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

Backends: Cloud Run Jobs (cloudrun), Cloud Run Functions (cloudrun-functions). Sim: `simulators/gcp/`. **16/16 ✓.**

| Service | Method | Used by | Sim status | Notes |
|---|---|---|---|---|
| Cloud Run | Jobs.CreateJob | cloudrun | ✓ | `registerCloudRunJobs` (cloudrunjobs.go:225) — full LRO + job metadata |
| Cloud Run | Jobs.DeleteJob | cloudrun | ✓ | (cloudrunjobs.go:317) — cascades execution delete |
| Cloud Run | Jobs.ListJobs | cloudrun | ✓ | (cloudrunjobs.go:302) — filters by project/location prefix |
| Cloud Run | Jobs.RunJob | cloudrun | ✓ | (cloudrunjobs.go:344) — creates execution with task metadata |
| Cloud Run | Executions.GetExecution | cloudrun | ✓ | (cloudrunjobs.go:539) — full execution state |
| Cloud Run | Executions.CancelExecution | cloudrun | ✓ | (cloudrunjobs.go:571) — stops container + injects cancel log |
| Cloud Run | Services.CreateService | cloudrun (UseService) | ✓ | v2 REST routes in `simulators/gcp/cloudrunservices.go::registerCloudRunServicesV2` covering Create/Get/List/Update/Delete on `/v2/projects/{p}/locations/{l}/services`. Returns proto-JSON shape `runpb.Service` expects (TerminalCondition=CONDITION_SUCCEEDED, LatestReadyRevision populated, generation as int64-string). |
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

Backends: Container Apps (aca), Azure Functions (azure-functions). Sim: `simulators/azure/`. **28/28 ✓.**

| Service | Method | Used by | Sim status | Notes |
|---|---|---|---|---|
| Container Apps | Jobs.BeginCreateOrUpdate | aca | ✓ | (containerapps.go:240) — full LRO + JobProperties + provisioningState=Succeeded |
| Container Apps | Jobs.BeginDelete | aca | ✓ | (containerapps.go:325) — cascades execution delete |
| Container Apps | Jobs.BeginStart | aca | ✓ | (containerapps.go:347) — execution metadata + LRO |
| Container Apps | Jobs.BeginStopExecution | aca | ✓ | (containerapps.go:592) |
| Container Apps | Jobs.NewListByResourceGroupPager | aca | ✓ | (containerapps.go:310) — pagination |
| Container Apps | ContainerApps.BeginCreateOrUpdate | aca (UseApp) | ✓ | `registerContainerAppsApps` in `simulators/azure/containerapps_apps.go`. Returns `provisioningState=Succeeded` + `LatestReadyRevisionName` + `LatestRevisionFqdn` so `appContainerState` reads "running" and `cloudServiceRegisterCNAME` can seed Private DNS. |
| Container Apps | ContainerApps.BeginDelete | aca (UseApp) | ✓ | `containerapps_apps.go` |
| Container Apps | ContainerApps.Get | aca (UseApp) | ✓ | `containerapps_apps.go` — backend reads `LatestRevisionFqdn` for CNAME registration |
| Container Apps | EnvStorages.CreateOrUpdate | aca | ✓ | (containerappsenv.go:210) |
| Container Apps | EnvStorages.Delete | aca | ✓ | (containerappsenv.go:254) |
| Container Apps | Executions.NewListPager | aca | ✓ | (containerapps.go:548) |
| Network | NSG.BeginCreateOrUpdate | aca | ✓ | (network.go:276) — SecurityRules + provisioningState |
| Network | NSG.BeginDelete | aca | ✓ | (network.go:329) |
| Network | NSG.Get | aca | ✓ | (network.go:313) |
| Network | NSGRules.BeginCreateOrUpdate | aca | ✓ | (network.go:353) |
| Private DNS | PrivateDNSZones.BeginCreateOrUpdate | aca | ✓ | (dns.go:84) |
| Private DNS | PrivateDNSZones.BeginDelete | aca | ✓ | (dns.go:176) |
| Private DNS | PrivateDNSZones.Get | aca | ✓ | (dns.go:132) |
| Private DNS | PrivateDNSRecords.CreateOrUpdate | aca | ✓ | (dns.go:192) — A + CNAME |
| Private DNS | PrivateDNSRecords.Delete | aca | ✓ | (dns.go:232) |
| Private DNS | PrivateDNSRecords.Get | aca | ✓ | (dns.go:212) |
| Storage | StorageAccounts.ListKeys | aca, azure-functions | ✓ | (files.go:335) |
| Log Analytics | Logs.QueryWorkspace | aca, azure-functions | ✓ | (monitor.go:349) — KQL parsing + Tables[0].Rows |
| Log Analytics | LogsHTTP.QueryWorkspace | aca | ✓ | (monitor.go:349) — HTTP fallback for non-TLS sim runs |
| App Service | WebApps.BeginCreateOrUpdate | azure-functions | ✓ | (functions.go:88) |
| App Service | WebApps.Delete | azure-functions | ✓ | (functions.go:178) |
| App Service | WebApps.NewListByResourceGroupPager | azure-functions | ✓ | (functions.go:163) |
| App Service | WebApps.UpdateAzureStorageAccounts | azure-functions | ✓ | `PUT /sites/{name}/config/azurestorageaccounts` in `simulators/azure/functions.go`. Round-trip of `AzureStoragePropertyDictionaryResource` matches `armappservice` wire format. |

## Closure tracking

All 77 rows (33 AWS + 16 GCP + 28 Azure) ship ✓. Standing rule: any new SDK call added to a backend must update this matrix and add a sim handler in the same commit (PLAN.md principle #10).
