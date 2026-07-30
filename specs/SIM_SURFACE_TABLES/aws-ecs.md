# Sim surface — aws-ecs

Surface registered in `simulators/aws/ecs.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `GET /ecs-exec/{sessionId}` | ✓ `simulators/aws/ecs.go:461::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /sockerless/tasks/{taskId}/archive` | ✓ `simulators/aws/ecs.go:467::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.CreateCluster` | ✓ `simulators/aws/ecs.go:425::handleECSCreateCluster` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.DescribeClusters` | ✓ `simulators/aws/ecs.go:426::handleECSDescribeClusters` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.UpdateCluster` | ✓ `simulators/aws/ecs.go:427::handleECSUpdateCluster` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.UpdateClusterSettings` | ✓ `simulators/aws/ecs.go:428::handleECSUpdateClusterSettings` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.RegisterTaskDefinition` | ✓ `simulators/aws/ecs.go:429::handleECSRegisterTaskDefinition` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.DeregisterTaskDefinition` | ✓ `simulators/aws/ecs.go:430::handleECSDeregisterTaskDefinition` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.DescribeTaskDefinition` | ✓ `simulators/aws/ecs.go:431::handleECSDescribeTaskDefinition` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.RunTask` | ✓ `simulators/aws/ecs.go:432::handleECSRunTask` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.DescribeTasks` | ✓ `simulators/aws/ecs.go:433::handleECSDescribeTasks` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.StopTask` | ✓ `simulators/aws/ecs.go:434::handleECSStopTask` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.ListTasks` | ✓ `simulators/aws/ecs.go:435::handleECSListTasks` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.DeleteCluster` | ✓ `simulators/aws/ecs.go:436::handleECSDeleteCluster` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.ListTagsForResource` | ✓ `simulators/aws/ecs.go:437::handleECSListTagsForResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.TagResource` | ✓ `simulators/aws/ecs.go:438::handleECSTagResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.UntagResource` | ✓ `simulators/aws/ecs.go:439::handleECSUntagResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.ExecuteCommand` | ✓ `simulators/aws/ecs.go:440::handleECSExecuteCommand` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.DeleteTaskDefinitions` | ✓ `simulators/aws/ecs.go:458::handleECSDeleteTaskDefinitions` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.PutAccountSetting` | ✓ `simulators/aws/ecs_account.go:30::handleECSPutAccountSetting` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.PutAccountSettingDefault` | ✓ `simulators/aws/ecs_account.go:31::handleECSPutAccountSettingDefault` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.DeleteAccountSetting` | ✓ `simulators/aws/ecs_account.go:32::handleECSDeleteAccountSetting` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.ListAccountSettings` | ✓ `simulators/aws/ecs_account.go:33::handleECSListAccountSettings` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.PutAttributes` | ✓ `simulators/aws/ecs_attributes.go:21::handleECSPutAttributes` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.DeleteAttributes` | ✓ `simulators/aws/ecs_attributes.go:22::handleECSDeleteAttributes` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.ListAttributes` | ✓ `simulators/aws/ecs_attributes.go:23::handleECSListAttributes` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.CreateCapacityProvider` | ✓ `simulators/aws/ecs_capacity.go:34::handleECSCreateCapacityProvider` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.DeleteCapacityProvider` | ✓ `simulators/aws/ecs_capacity.go:35::handleECSDeleteCapacityProvider` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.UpdateCapacityProvider` | ✓ `simulators/aws/ecs_capacity.go:36::handleECSUpdateCapacityProvider` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.RegisterContainerInstance` | ✓ `simulators/aws/ecs_container_instances.go:61::handleECSRegisterContainerInstance` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.DeregisterContainerInstance` | ✓ `simulators/aws/ecs_container_instances.go:62::handleECSDeregisterContainerInstance` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.DescribeContainerInstances` | ✓ `simulators/aws/ecs_container_instances.go:63::handleECSDescribeContainerInstances` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.ListContainerInstances` | ✓ `simulators/aws/ecs_container_instances.go:64::handleECSListContainerInstances` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.UpdateContainerInstancesState` | ✓ `simulators/aws/ecs_container_instances.go:65::handleECSUpdateContainerInstancesState` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.UpdateContainerAgent` | ✓ `simulators/aws/ecs_container_instances.go:66::handleECSUpdateContainerAgent` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.SubmitContainerStateChange` | ✓ `simulators/aws/ecs_container_instances.go:67::handleECSSubmitContainerStateChange` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.SubmitTaskStateChange` | ✓ `simulators/aws/ecs_container_instances.go:68::handleECSSubmitTaskStateChange` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.SubmitAttachmentStateChanges` | ✓ `simulators/aws/ecs_container_instances.go:69::handleECSSubmitAttachmentStateChanges` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.DiscoverPollEndpoint` | ✓ `simulators/aws/ecs_container_instances.go:70::handleECSDiscoverPollEndpoint` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.CreateDaemon` | ✓ `simulators/aws/ecs_daemons.go:89::handleECSCreateDaemon` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.DeleteDaemon` | ✓ `simulators/aws/ecs_daemons.go:90::handleECSDeleteDaemon` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.DescribeDaemon` | ✓ `simulators/aws/ecs_daemons.go:91::handleECSDescribeDaemon` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.UpdateDaemon` | ✓ `simulators/aws/ecs_daemons.go:92::handleECSUpdateDaemon` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.ListDaemons` | ✓ `simulators/aws/ecs_daemons.go:93::handleECSListDaemons` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.DescribeDaemonDeployments` | ✓ `simulators/aws/ecs_daemons.go:94::handleECSDescribeDaemonDeployments` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.ListDaemonDeployments` | ✓ `simulators/aws/ecs_daemons.go:95::handleECSListDaemonDeployments` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.DescribeDaemonRevisions` | ✓ `simulators/aws/ecs_daemons.go:96::handleECSDescribeDaemonRevisions` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.RegisterDaemonTaskDefinition` | ✓ `simulators/aws/ecs_daemons.go:97::handleECSRegisterDaemonTaskDefinition` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.DeleteDaemonTaskDefinition` | ✓ `simulators/aws/ecs_daemons.go:98::handleECSDeleteDaemonTaskDefinition` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.DescribeDaemonTaskDefinition` | ✓ `simulators/aws/ecs_daemons.go:99::handleECSDescribeDaemonTaskDefinition` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.ListDaemonTaskDefinitions` | ✓ `simulators/aws/ecs_daemons.go:100::handleECSListDaemonTaskDefinitions` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.CreateExpressGatewayService` | ✓ `simulators/aws/ecs_express.go:128::handleECSCreateExpressGatewayService` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.DescribeExpressGatewayService` | ✓ `simulators/aws/ecs_express.go:129::handleECSDescribeExpressGatewayService` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.UpdateExpressGatewayService` | ✓ `simulators/aws/ecs_express.go:130::handleECSUpdateExpressGatewayService` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.DeleteExpressGatewayService` | ✓ `simulators/aws/ecs_express.go:131::handleECSDeleteExpressGatewayService` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.CreateService` | ✓ `simulators/aws/ecs_service.go:88::handleECSCreateService` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.DescribeServices` | ✓ `simulators/aws/ecs_service.go:89::handleECSDescribeServices` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.ListServices` | ✓ `simulators/aws/ecs_service.go:90::handleECSListServices` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.UpdateService` | ✓ `simulators/aws/ecs_service.go:91::handleECSUpdateService` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.DeleteService` | ✓ `simulators/aws/ecs_service.go:92::handleECSDeleteService` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.PutClusterCapacityProviders` | ✓ `simulators/aws/ecs_service.go:93::handleECSPutClusterCapacityProviders` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.ListClusters` | ✓ `simulators/aws/ecs_service.go:94::handleECSListClusters` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.ListTaskDefinitions` | ✓ `simulators/aws/ecs_service.go:95::handleECSListTaskDefinitions` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.ListTaskDefinitionFamilies` | ✓ `simulators/aws/ecs_service.go:96::handleECSListTaskDefinitionFamilies` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.DescribeCapacityProviders` | ✓ `simulators/aws/ecs_service.go:97::handleECSDescribeCapacityProviders` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.DescribeServiceDeployments` | ✓ `simulators/aws/ecs_service_deployments.go:57::handleECSDescribeServiceDeployments` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.ListServiceDeployments` | ✓ `simulators/aws/ecs_service_deployments.go:58::handleECSListServiceDeployments` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.DescribeServiceRevisions` | ✓ `simulators/aws/ecs_service_deployments.go:59::handleECSDescribeServiceRevisions` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.StopServiceDeployment` | ✓ `simulators/aws/ecs_service_deployments.go:60::handleECSStopServiceDeployment` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.ContinueServiceDeployment` | ✓ `simulators/aws/ecs_service_deployments.go:61::handleECSContinueServiceDeployment` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.ListServicesByNamespace` | ✓ `simulators/aws/ecs_service_deployments.go:62::handleECSListServicesByNamespace` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.StartTask` | ✓ `simulators/aws/ecs_start_task.go:19::handleECSStartTask` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.GetTaskProtection` | ✓ `simulators/aws/ecs_task_protection.go:29::handleECSGetTaskProtection` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.UpdateTaskProtection` | ✓ `simulators/aws/ecs_task_protection.go:30::handleECSUpdateTaskProtection` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.CreateTaskSet` | ✓ `simulators/aws/ecs_tasksets.go:52::handleECSCreateTaskSet` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.DeleteTaskSet` | ✓ `simulators/aws/ecs_tasksets.go:53::handleECSDeleteTaskSet` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.DescribeTaskSets` | ✓ `simulators/aws/ecs_tasksets.go:54::handleECSDescribeTaskSets` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.UpdateTaskSet` | ✓ `simulators/aws/ecs_tasksets.go:55::handleECSUpdateTaskSet` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.UpdateServicePrimaryTaskSet` | ✓ `simulators/aws/ecs_tasksets.go:56::handleECSUpdateServicePrimaryTaskSet` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->

## ECS Express Mode (Express Gateway services)

The four Express Gateway operations are registered in `simulators/aws/ecs_express.go`
(`CreateExpressGatewayService`, `DescribeExpressGatewayService`,
`UpdateExpressGatewayService`, `DeleteExpressGatewayService`). Each composes the real
backing resources (ECS Fargate service, ELBv2 ALB/target-group/listener, ACM cert, EC2
security group, Application Auto Scaling target + policy) so they are describable through
their own service slices. See [`docs/ECS_EXPRESS_MODE.md`](../../docs/ECS_EXPRESS_MODE.md)
for the full API, the Express-vs-vanilla-ECS comparison, and the assembly details.

<!-- HAND-WRITTEN END -->
