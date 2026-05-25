# Sim surface — aws-ecs

Surface registered in `simulators/aws/ecs.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with a BUG / deferred-subtask reference; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no terraform-provider resource for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `GET /ecs-exec/{sessionId}` | ✓ `simulators/aws/ecs.go:244::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `PUT /sockerless/tasks/{taskId}/archive` | ✓ `simulators/aws/ecs.go:250::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.CreateCluster` | ✓ `simulators/aws/ecs.go:228::handleECSCreateCluster` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.DescribeClusters` | ✓ `simulators/aws/ecs.go:229::handleECSDescribeClusters` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.RegisterTaskDefinition` | ✓ `simulators/aws/ecs.go:230::handleECSRegisterTaskDefinition` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.DeregisterTaskDefinition` | ✓ `simulators/aws/ecs.go:231::handleECSDeregisterTaskDefinition` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.DescribeTaskDefinition` | ✓ `simulators/aws/ecs.go:232::handleECSDescribeTaskDefinition` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.RunTask` | ✓ `simulators/aws/ecs.go:233::handleECSRunTask` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.DescribeTasks` | ✓ `simulators/aws/ecs.go:234::handleECSDescribeTasks` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.StopTask` | ✓ `simulators/aws/ecs.go:235::handleECSStopTask` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.ListTasks` | ✓ `simulators/aws/ecs.go:236::handleECSListTasks` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.DeleteCluster` | ✓ `simulators/aws/ecs.go:237::handleECSDeleteCluster` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.ListTagsForResource` | ✓ `simulators/aws/ecs.go:238::handleECSListTagsForResource` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.TagResource` | ✓ `simulators/aws/ecs.go:239::handleECSTagResource` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.UntagResource` | ✓ `simulators/aws/ecs.go:240::handleECSUntagResource` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.ExecuteCommand` | ✓ `simulators/aws/ecs.go:241::handleECSExecuteCommand` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |

## Open subtasks staged forward

- sdk-test / tf-test columns are ✗-with-deferral for every row above. Each subsequent surface-touching PR fills in the column for the rows it covers; remaining ✗s are tracked under BUG-1159 (paged-iterator sweep) + BUG-1147 (tf-test parity sweep).
- Missing ops (not in HandleFunc but documented by the cloud provider) get ✗ rows added when a community-filed issue surfaces them or a periodic audit lands a sweep.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
