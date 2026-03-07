# ECS Backend: Delegate Method Implementation Plan

## Overview

The ECS backend implements `api.Backend` (65 methods). Currently **14 methods** have real cloud-native implementations in `backend_impl.go`:

- `ContainerCreate`, `ContainerStart`, `ContainerStop`, `ContainerKill`, `ContainerRemove`
- `ContainerLogs`, `ContainerRestart`, `ContainerPrune`, `ContainerPause`, `ContainerUnpause`
- `ImagePull`, `ImageLoad`
- `VolumeRemove`, `VolumePrune`

The remaining **51 methods** in `backend_delegates_gen.go` delegate to `s.BaseServer.Method()`.

## Priority Summary

| Priority | Count | Description |
|----------|-------|-------------|
| P0 | 1 | BaseServer implementation is actively wrong for ECS |
| P1 | 19 | Works but misses cloud-specific features |
| P2 | 31 | BaseServer implementation is adequate |

---

## P0 — Critical (1 method)

### ExecStart
- **BaseServer behavior**: Runs command via `s.Drivers.Exec.Exec()` using the process/synthetic driver. Executes locally, not inside the Fargate task.
- **Why wrong**: For ECS containers without a connected agent, the command executes locally on the backend host, not inside the remote Fargate task.
- **Real implementation**: Check `c.AgentAddress`. If set, delegate to BaseServer (agent driver handles it). If no agent, use `ecs:ExecuteCommand` API with SSM Session Manager for I/O streaming.
- **AWS APIs**: `ecs:ExecuteCommand`, SSM Session Manager WebSocket
- **Dependencies**: Requires `enableExecuteCommand: true` in task definition. Add to `registerTaskDefinition`.

---

## P1 — Important (19 methods)

### Container Lifecycle

#### ContainerInspect
- **BaseServer**: Returns container from in-memory store only.
- **Why incomplete**: Does not reflect real ECS task status. A task could have stopped or changed IP without the backend knowing.
- **Implementation**: Call `ecs:DescribeTasks` with stored TaskARN. Merge real task status, exit code, network interface IP into in-memory container. Fall back to in-memory if no TaskARN.
- **AWS APIs**: `ecs:DescribeTasks`

#### ContainerList
- **BaseServer**: Lists from in-memory store.
- **Why incomplete**: Running containers whose tasks have stopped still show as running.
- **Implementation**: Batch `ecs:DescribeTasks` (up to 100 per request) to refresh status before filtering. Factor refresh into shared `refreshTaskStatus()` helper.
- **AWS APIs**: `ecs:DescribeTasks` (batched)

#### ContainerWait
- **BaseServer**: Blocks on in-memory channel.
- **Why incomplete**: If `pollTaskExit` goroutine fails, channel never closes.
- **Implementation**: Add `ecs.NewTasksStoppedWaiter` as fallback alongside existing channel. Provides redundancy.
- **AWS APIs**: `ecs:DescribeTasks` (via waiter)

#### ContainerTop
- **BaseServer**: Returns synthetic single-process listing.
- **Why incomplete**: Real ECS tasks may run multiple processes with ECS Exec.
- **Implementation**: If ECS Execute Command is enabled, run `ps aux` via `ecs:ExecuteCommand` and parse output. Fall back to synthetic response otherwise.
- **AWS APIs**: `ecs:ExecuteCommand`

#### ContainerStats
- **BaseServer**: Returns synthetic stats with fake values.
- **Why incomplete**: Does not reflect real Fargate resource usage.
- **Implementation**: Query CloudWatch Container Insights (`CpuUtilized`, `MemoryUtilized`, `NetworkRxBytes`, `NetworkTxBytes`). Poll every 5s for streaming.
- **AWS APIs**: `cloudwatch:GetMetricData` (Container Insights)
- **Dependencies**: Container Insights must be enabled on cluster.

### Exec

#### ExecCreate
- **BaseServer**: Creates exec instance in-memory.
- **Enhancement**: Add validation — if no agent AND ECS Execute Command not enabled, return error early.

### Images

#### ImageBuild
- **BaseServer**: Builds synthetic image locally.
- **Why incomplete**: Cannot build images on Fargate.
- **Implementation (short-term)**: Return `NotImplementedError`.
- **Implementation (future)**: Submit to CodeBuild, push to ECR.
- **AWS APIs (future)**: `codebuild:StartBuild`, `ecr:CreateRepository`

#### ImagePush
- **BaseServer**: Returns synthetic "pushed" progress.
- **Why incomplete**: Does not push to ECR.
- **Recommendation**: Keep as P2 unless ImageBuild is implemented.

### Networks

#### NetworkCreate
- **BaseServer**: In-memory synthetic network.
- **Why incomplete**: No real AWS networking resources.
- **Implementation**: Create Security Group (`ec2:CreateSecurityGroup`) and optionally Cloud Map namespace. Store SG ID in `NetworkState`. Use network's SG when starting containers.
- **AWS APIs**: `ec2:CreateSecurityGroup`, `ec2:AuthorizeSecurityGroupIngress`, `servicediscovery:CreatePrivateDnsNamespace`

#### NetworkInspect
- **BaseServer**: Returns in-memory network.
- **Implementation**: Enrich with SG details from `ec2:DescribeSecurityGroups` and subnet CIDR info.

#### NetworkConnect / NetworkDisconnect
- **BaseServer**: In-memory only.
- **Implementation**: Record SG association in ECSState. Takes effect on next ContainerStart (Fargate ENIs cannot be modified after launch).

#### NetworkRemove / NetworkPrune
- **BaseServer**: In-memory only.
- **Implementation**: Delete SG via `ec2:DeleteSecurityGroup` and namespace via `servicediscovery:DeleteNamespace`.

### Volumes

#### VolumeCreate
- **BaseServer**: Creates local temp directory.
- **Why incomplete**: Fargate cannot use local volumes. Needs EFS.
- **Implementation**: Create EFS Access Point (`efs:CreateAccessPoint`). Store access point ID in `VolumeState`. Wire into `buildContainerDef` as EFS volume.
- **AWS APIs**: `efs:CreateAccessPoint`

### Pods

#### PodStart
- **BaseServer**: Marks containers as running in-memory only.
- **Why incomplete**: Does not launch an ECS task.
- **Implementation**: Collect pod containers, call `startMultiContainerTaskTyped` to create combined task definition and run task.

#### PodStop / PodKill
- **BaseServer**: Force-stops in-memory only.
- **Implementation**: Look up shared TaskARN, call `ecs:StopTask`, then update in-memory state.

#### PodRemove
- **BaseServer**: Removes from in-memory store only.
- **Implementation**: Deregister task definitions, stop running tasks, mark cleaned up, then remove from store.
- **AWS APIs**: `ecs:StopTask`, `ecs:DeregisterTaskDefinition`

### System

#### Info
- **BaseServer**: Returns in-memory counts with static descriptor.
- **Implementation**: Call `ecs:DescribeClusters` for `runningTasksCount` and cluster capacity. Merge with in-memory counts.

---

## P2 — Acceptable (31 methods)

These methods work correctly with BaseServer delegation:

- **Container**: Attach, Rename, Resize, Update, Changes, Export, Commit, StatPath, GetArchive, PutArchive
- **Exec**: Inspect, Resize
- **Images**: Inspect, List, Remove, History, Prune, Save, Search, Tag
- **Networks**: List
- **Volumes**: List, Inspect
- **Pods**: Create, List, Inspect, Exists
- **System**: Df, Events
- **Auth**: Login

---

## Implementation Phases

### Phase A: Task Status Reconciliation (3 methods)
Create `refreshTaskStatus()` helper. Override ContainerInspect, ContainerList, ContainerWait.
**Effort**: Medium

### Phase B: ECS Execute Command / Exec (2 methods)
Add `EnableExecuteCommand` to config and task definition. Override ExecCreate (validation), ExecStart (ECS Execute Command fallback).
**Effort**: High (SSM WebSocket client)

### Phase C: Network Isolation (6 methods)
Override NetworkCreate/Remove/Prune/Connect/Disconnect/Inspect. Create Security Groups, wire into task launch.
**Effort**: Medium

### Phase D: EFS Volumes (1 method)
Override VolumeCreate. Create EFS Access Points, wire into task definitions.
**Effort**: Medium

### Phase E: Pod Lifecycle (4 methods)
Override PodStart/Stop/Kill/Remove. Use existing multi-container infrastructure.
**Effort**: Medium

### Phase F: System Info and Stats (2 methods)
Override Info (DescribeClusters), ContainerStats (Container Insights).
**Effort**: Low

### Phase G: Image Build (1 method)
Override ImageBuild → NotImplementedError. Future: CodeBuild integration.
**Effort**: Low

### Recommended Order
A → E → D → C → F → B → G
