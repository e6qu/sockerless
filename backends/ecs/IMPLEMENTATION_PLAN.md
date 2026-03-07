# ECS Backend: Delegate Method Implementation Plan

## Overview

The ECS backend implements `api.Backend` (65 methods). Currently **30 methods** have real cloud-native implementations in `backend_impl.go`:

- `ContainerCreate`, `ContainerStart`, `ContainerStop`, `ContainerKill`, `ContainerRemove`
- `ContainerLogs`, `ContainerRestart`, `ContainerPrune`, `ContainerPause`, `ContainerUnpause`
- `ContainerInspect` (**DONE** — refreshes task status via ecs:DescribeTasks before returning)
- `ContainerList` (**DONE** — batch-refreshes all containers with TaskARNs via ecs:DescribeTasks)
- `ContainerExport` (**DONE** — returns NotImplementedError, no filesystem access on Fargate)
- `ContainerCommit` (**DONE** — returns NotImplementedError, cannot snapshot Fargate containers)
- `ImagePull` (**DONE** — delegates to `s.images.Pull`; ECR auth via `ECRAuthProvider.GetToken`)
- `ImagePush` (**DONE** — delegates to `s.images.Push`; ECR sync via `ECRAuthProvider.OnPush`)
- `ImageTag` (**DONE** — delegates to `s.images.Tag`; ECR sync via `ECRAuthProvider.OnTag`)
- `ImageRemove` (**DONE** — delegates to `s.images.Remove`; ECR cleanup via `ECRAuthProvider.OnRemove`)
- `ImageLoad` (**DONE** — delegates to BaseServer via `s.images.Load`)
- `ImageBuild`
- `VolumeRemove`, `VolumePrune`
- `ExecStart` (**DONE** — agent check, delegates or returns NotImplementedError)
- `ExecCreate` (**DONE** — validates agent connectivity before creating exec instance)
- `PodStart` (**DONE** — calls ContainerStart per container)
- `PodStop` (**DONE** — calls ContainerStop per container)
- `PodKill` (**DONE** — calls ContainerKill per container)
- `PodRemove` (**DONE** — calls ContainerRemove per container, checks running when !force, deletes pod)
- `Info` (**DONE** — enriches with ecs:DescribeClusters data, non-fatal on AWS errors)

The remaining **35 methods** in `backend_delegates_gen.go` delegate to `s.BaseServer.Method()`.

## Priority Summary

| Priority | Count | Description |
|----------|-------|-------------|
| P0 | ~~1~~ 0 | BaseServer implementation is actively wrong for ECS |
| P1 | ~~19~~ ~~14~~ ~~9~~ 6 | Works but misses cloud-specific features |
| P2 | ~~31~~ 26 | BaseServer implementation is adequate |

---

## P0 — Critical (1 method)

### ExecStart — DONE
- **Implementation**: Checks agent connection. If agent connected, delegates to BaseServer. If no agent, returns `NotImplementedError` (ECS ExecuteCommand/SSM not yet supported).
- ~~**BaseServer behavior**: Runs command via `s.Drivers.Exec.Exec()` using the process/synthetic driver. Executes locally, not inside the Fargate task.~~
- **Future**: Add `ecs:ExecuteCommand` API with SSM Session Manager for I/O streaming when no agent is connected.

---

## P1 — Important (19 methods)

### Container Lifecycle

#### ContainerInspect — DONE
- **Implementation**: Calls `refreshTaskStatus(id)` which does `ecs:DescribeTasks` with stored TaskARN. `applyTaskStatus()` merges real task status (STOPPED → exited with exit code, RUNNING → IP backfill). Falls back to in-memory if no TaskARN or AWS error (non-fatal). Then delegates to `BaseServer.ContainerInspect()`.
- **AWS APIs**: `ecs:DescribeTasks`

#### ContainerList — DONE
- **Implementation**: Collects all container IDs with TaskARNs, calls `refreshTaskStatusBatch(ids)` which batches `ecs:DescribeTasks` (100 ARNs per call). Each task result is applied via `applyTaskStatus()`. AWS errors are non-fatal (skipped per batch). Then delegates to `BaseServer.ContainerList()`.
- **AWS APIs**: `ecs:DescribeTasks` (batched, up to 100 per call)
- **Note**: In pod scenarios, multiple containers may share one TaskARN; currently only the first container per TaskARN is refreshed in the batch map. Non-critical since all pod containers share the same task lifecycle.

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

#### ExecCreate — DONE
- **Implementation**: Resolves container, checks running state, validates agent connectivity (`c.AgentAddress != ""`). Returns `NotImplementedError` if no agent attached. Delegates to `BaseServer.ExecCreate()` when agent is available.

### Images

#### ImageBuild — DONE
- **Implementation**: Returns `NotImplementedError` ("ECS backend does not support image build; push pre-built images to ECR").
- **Future**: Submit to CodeBuild, push to ECR (`codebuild:StartBuild`, `ecr:CreateRepository`).

#### ImagePush — DONE (full ECR integration)
- **Implementation**: Delegates to `s.images.Push` (unified ImageManager). ECR sync handled by `ECRAuthProvider.OnPush` for ECR targets. Returns progress stream for all targets. ECR failures are non-fatal.
- **AWS APIs**: `ecr:CreateRepository`, `ecr:PutImage` (via `ECRAuthProvider`)

#### ImageTag — DONE
- **Implementation**: Delegates to `s.images.Tag` (unified ImageManager). ECR sync handled by `ECRAuthProvider.OnTag` (best-effort, non-fatal).
- **AWS APIs**: `ecr:CreateRepository`, `ecr:PutImage` (via `ECRAuthProvider`)

#### ImageRemove — DONE
- **Implementation**: Delegates to `s.images.Remove` (unified ImageManager). ECR cleanup handled by `ECRAuthProvider.OnRemove` (best-effort, non-fatal).
- **AWS APIs**: `ecr:BatchDeleteImage` (via `ECRAuthProvider`)

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

#### PodStart — DONE
- **Implementation**: Iterates pod containers, calls `s.ContainerStart(cid)` for each non-running container, collects errors, sets pod status to "running" on success.

#### PodStop — DONE
- **Implementation**: Iterates pod containers, calls `s.ContainerStop(cid, timeout)` for each running container, filters out NotModifiedError, sets pod status to "stopped".

#### PodKill — DONE
- **Implementation**: Iterates pod containers, calls `s.ContainerKill(cid, signal)` for each running container (defaults signal to SIGKILL), sets pod status to "exited".

#### PodRemove — DONE
- **Implementation**: Checks for running containers when `force=false` (returns ConflictError). Calls `s.ContainerRemove(cid, force)` for each container (handles ECS task stop, task def deregister, registry cleanup). Deletes pod at the end.

### System

#### Info — DONE
- **Implementation**: Gets base info from BaseServer, enriches with `ecs:DescribeClusters` (running task count). AWS errors are non-fatal (logs warning, returns base info).

---

## P2 — Acceptable (31 methods)

These methods work correctly with BaseServer delegation:

- **Container**: Attach, Rename, Resize, Update, Changes, StatPath, GetArchive, PutArchive
- **Container (now DONE with NotImplementedError)**: Export, Commit
- **Exec**: Inspect, Resize
- **Images**: Inspect, List, History, Prune, Save, Search (all via `s.images.*` unified ImageManager)
- **Networks**: List
- **Volumes**: List, Inspect
- **Pods**: Create, List, Inspect, Exists
- **System**: Df, Events
- **Auth**: Login

---

## Implementation Phases

### Phase A: Task Status Reconciliation (3 methods) — ContainerInspect + ContainerList DONE
Created `refreshTaskStatus()`, `refreshTaskStatusBatch()`, and `applyTaskStatus()` helpers in `containers.go`. ContainerInspect and ContainerList override BaseServer with task status refresh. ContainerWait (ECS TasksStoppedWaiter fallback) remains future work.
**Effort**: Medium

### Phase B: ECS Execute Command / Exec (2 methods) — ExecStart + ExecCreate DONE
ExecStart implemented with agent check + NotImplementedError fallback. ExecCreate validates agent connectivity before creating exec instance. Full ECS ExecuteCommand/SSM integration remains future work.
**Effort**: High (SSM WebSocket client) for full implementation

### Phase C: Network Isolation (6 methods)
Override NetworkCreate/Remove/Prune/Connect/Disconnect/Inspect. Create Security Groups, wire into task launch.
**Effort**: Medium

### Phase D: EFS Volumes (1 method)
Override VolumeCreate. Create EFS Access Points, wire into task definitions.
**Effort**: Medium

### Phase E: Pod Lifecycle (4 methods) — DONE
PodStart/Stop/Kill/Remove all implemented. Each delegates to the corresponding Container method per pod member, with proper error collection, force checks, and pod status updates.
**Effort**: Medium

### Phase F: System Info and Stats (2 methods) — Info DONE
Info implemented (DescribeClusters enrichment, non-fatal on AWS errors). ContainerStats (Container Insights) remains future work.
**Effort**: Low

### Phase G: Image Build + Push + Export + Commit (4 methods) — DONE
ImageBuild, ContainerExport, ContainerCommit return `NotImplementedError` with descriptive messages. ImagePush now has full ECR integration (CreateRepository + PutImage for ECR targets).
**Effort**: Low

### Phase H: ECR Image Management (3 methods) — DONE (unified)
All 12 image methods now delegate to `s.images` (core `ImageManager`). ECR integration via `ECRAuthProvider` in `image_auth.go` (implements `core.AuthProvider`): `GetToken` for ECR auth, `OnPush`/`OnTag` for ECR recording, `OnRemove` for ECR cleanup. Old `registry.go` (with `fetchImageConfig`, `parseImageRef`, `getECRToken`, `getDockerHubToken`, `recordImageInECR`, `isECRRegistry`) deleted — replaced by unified image management.
**Effort**: Medium

### Recommended Order (remaining)
A(ContainerWait) → D → C → F(ContainerStats)

### Completed Phases
A(ContainerInspect, ContainerList) → B(ExecStart, ExecCreate) → E(Pod lifecycle) → F(Info) → G(ImageBuild, Export, Commit) → H(Unified image management via ImageManager + ECRAuthProvider)
