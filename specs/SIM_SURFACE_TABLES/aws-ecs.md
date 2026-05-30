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
| `GET /ecs-exec/{sessionId}` | ✓ `simulators/aws/ecs.go:244::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /sockerless/tasks/{taskId}/archive` | ✓ `simulators/aws/ecs.go:250::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.CreateCluster` | ✓ `simulators/aws/ecs.go:228::handleECSCreateCluster` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.DescribeClusters` | ✓ `simulators/aws/ecs.go:229::handleECSDescribeClusters` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.RegisterTaskDefinition` | ✓ `simulators/aws/ecs.go:230::handleECSRegisterTaskDefinition` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.DeregisterTaskDefinition` | ✓ `simulators/aws/ecs.go:231::handleECSDeregisterTaskDefinition` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.DescribeTaskDefinition` | ✓ `simulators/aws/ecs.go:232::handleECSDescribeTaskDefinition` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.RunTask` | ✓ `simulators/aws/ecs.go:233::handleECSRunTask` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.DescribeTasks` | ✓ `simulators/aws/ecs.go:234::handleECSDescribeTasks` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.StopTask` | ✓ `simulators/aws/ecs.go:235::handleECSStopTask` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.ListTasks` | ✓ `simulators/aws/ecs.go:236::handleECSListTasks` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.DeleteCluster` | ✓ `simulators/aws/ecs.go:237::handleECSDeleteCluster` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.ListTagsForResource` | ✓ `simulators/aws/ecs.go:238::handleECSListTagsForResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.TagResource` | ✓ `simulators/aws/ecs.go:239::handleECSTagResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.UntagResource` | ✓ `simulators/aws/ecs.go:240::handleECSUntagResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerServiceV20141113.ExecuteCommand` | ✓ `simulators/aws/ecs.go:241::handleECSExecuteCommand` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
