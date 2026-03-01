# AWS Backend Gap Analysis (ECS + Lambda)

Comparing the backends against:
1. **Docker API coverage** — what Docker operations work end-to-end on cloud
2. **Cloud SDK feature usage** — what AWS features exist but aren't leveraged

---

## Part A: Docker API Coverage on ECS Backend

### 1. Container Lifecycle

| Docker API | ECS Backend | Status | Notes |
|---|---|---|---|
| `POST /containers/create` | Creates ECS task definition | **FULL** | Maps image, cmd, env, volumes, ports, working dir, user |
| `POST /containers/{id}/start` | Calls `RunTask` on Fargate | **FULL** | Waits for RUNNING state, extracts ENI IP |
| `POST /containers/{id}/stop` | Calls `StopTask` | **FULL** | |
| `POST /containers/{id}/kill` | Calls `StopTask` with reason | **FULL** | All signals → StopTask (no granular signal support) |
| `DELETE /containers/{id}` | StopTask + DeregisterTaskDefinition | **FULL** | |
| `POST /containers/{id}/wait` | Polls DescribeTasks for STOPPED | **FULL** | Returns exit code from container |
| `POST /containers/{id}/restart` | Handled by core (stop + start) | **FULL** | |
| `POST /containers/{id}/pause` | **NOT SUPPORTED** | **GAP** | ECS Fargate has no pause/unpause concept |
| `POST /containers/{id}/unpause` | **NOT SUPPORTED** | **GAP** | Same |
| `POST /containers/{id}/update` | **NOT SUPPORTED** | **GAP** | Cannot update task def in-place on running task |
| `POST /containers/{id}/rename` | Handled by core (local state only) | **PARTIAL** | Renames locally but ECS task name unchanged |

### 2. Container Inspection & Streaming

| Docker API | ECS Backend | Status | Notes |
|---|---|---|---|
| `GET /containers/{id}/json` | Handled by core (local state) | **FULL** | Returns stored container config |
| `GET /containers/json` | Handled by core | **FULL** | Lists containers from store |
| `GET /containers/{id}/logs` | Override: reads CloudWatch Logs | **FULL** | Supports follow, tail, timestamps |
| `POST /containers/{id}/attach` | **Via agent only** | **PARTIAL** | Requires forward/reverse agent connection; no attach without agent |
| `GET /containers/{id}/top` | **Via agent only** | **PARTIAL** | Returns synthetic list if no agent |
| `GET /containers/{id}/stats` | **Via agent only** | **PARTIAL** | Returns synthetic stats if no agent |
| `GET /containers/{id}/changes` | Stubbed (empty list) | **STUB** | No filesystem tracking on ECS |
| `GET /containers/{id}/export` | **Via agent only** | **PARTIAL** | Returns empty tar if no agent |

### 3. Exec

| Docker API | ECS Backend | Status | Notes |
|---|---|---|---|
| `POST /containers/{id}/exec` | Creates exec in store | **FULL** | |
| `POST /exec/{id}/start` | **Via agent only** | **PARTIAL** | Requires agent connection; returns error without agent |
| `GET /exec/{id}/json` | Handled by core | **FULL** | |

### 4. Images

| Docker API | ECS Backend | Status | Notes |
|---|---|---|---|
| `POST /images/create` (pull) | Override: may use ECR auth | **FULL** | Handles ECR token exchange, registry auth |
| All other image ops | Handled by core (synthetic) | **FULL** | |

### 5. Networks, Volumes, System

All handled by core defaults. No ECS-specific overrides. Fully functional
within the core's synthetic/WASM model.

### 6. Docker API Operations Not Possible on ECS

These Docker API features have **no meaningful ECS equivalent**:

| Feature | Why |
|---|---|
| `pause` / `unpause` | Fargate tasks cannot be paused |
| `update` (resource limits) | Cannot change CPU/memory on running task |
| `commit` (create image from container) | No filesystem snapshot capability |
| Signal-specific `kill` | Only SIGKILL (StopTask); no SIGTERM/SIGUSR1/etc. |
| Filesystem `changes` | No overlay filesystem tracking |
| Privileged mode | Fargate doesn't support `--privileged` |
| Host networking | Fargate is always `awsvpc` mode |
| Device access | No `--device` support |
| `--pid`, `--ipc`, `--uts` namespace sharing | Fargate manages namespaces |

---

## Part B: Docker API Coverage on Lambda Backend

### 1. Container Lifecycle

| Docker API | Lambda Backend | Status | Notes |
|---|---|---|---|
| `POST /containers/create` | Calls `CreateFunction` | **FULL** | Maps image, env, memory, timeout |
| `POST /containers/{id}/start` | Calls `Invoke` (async goroutine) | **FULL** | Fire-and-forget; no ongoing process monitoring |
| `POST /containers/{id}/stop` | No-op (function runs to completion) | **PARTIAL** | Cannot stop a running Lambda invocation |
| `POST /containers/{id}/kill` | Disconnects agent | **PARTIAL** | Only disconnects agent; function may continue running |
| `DELETE /containers/{id}` | Calls `DeleteFunction` | **FULL** | |
| `POST /containers/{id}/wait` | Handled by core | **PARTIAL** | Wait for agent disconnect, not actual function completion |
| `POST /containers/{id}/restart` | Handled by core | **PARTIAL** | |
| `POST /containers/{id}/pause` | **NOT SUPPORTED** | **GAP** | |
| `POST /containers/{id}/unpause` | **NOT SUPPORTED** | **GAP** | |

### 2. Container Inspection & Streaming

| Docker API | Lambda Backend | Status | Notes |
|---|---|---|---|
| `GET /containers/{id}/logs` | Override: reads CloudWatch Logs | **FULL** | First discovers latest log stream, then reads events |
| `POST /containers/{id}/attach` | **Via agent only** | **PARTIAL** | Agent-dependent |
| `GET /containers/{id}/top` | Synthetic | **STUB** | No process visibility |
| `GET /containers/{id}/stats` | Synthetic | **STUB** | No resource stats |
| Exec operations | **Via agent only** | **PARTIAL** | Agent-dependent |

### 3. Docker API Operations Not Possible on Lambda

| Feature | Why |
|---|---|
| `stop` | Cannot stop a running invocation (only waits for timeout) |
| `pause` / `unpause` | Not applicable |
| `attach` (without agent) | Lambda doesn't expose TTY/stdin |
| Persistent state | Lambda is stateless — no volumes, no filesystem persistence |
| Port mapping | Lambda uses event-driven invocation, not network listeners |
| Long-running processes | Max 15 minutes, no keep-alive |
| Container-to-container networking | No VPC-to-container direct access |

---

## Part C: Cloud SDK Feature Gaps

### 1. ECS Features Available But Not Used

| AWS Feature | Description | Could Enable |
|---|---|---|
| **ECS Services** | Long-running, auto-restarting, load-balanced containers | Persistent container workloads (not just one-shot tasks) |
| **Service Auto Scaling** | Target tracking / step scaling policies | Scale containers based on load |
| **ECS Exec** | Native `aws ecs execute-command` (uses SSM Agent) | `docker exec` without custom agent |
| **Container Insights** | CloudWatch Container Insights for metrics | Better `docker stats` data |
| **Capacity Providers** | Fargate Spot, EC2 capacity management | Cost optimization |
| **Task Sets** | Blue/green deployments | Canary deployments |
| **Service Connect** | Service-to-service networking with Cloud Map | Container DNS resolution |
| **EFS volume mounts** | Native EFS integration | Persistent volumes without agent |
| **Secrets Manager integration** | Inject secrets as env vars or files | Secure secret management |
| **FireLens** | Custom log routing (Fluentd/Fluent Bit sidecar) | Advanced log routing |
| **GPU support** | EC2 launch type with GPU instances | GPU workloads |
| **gRPC health checks** | Container-level health checking | Docker HEALTHCHECK equivalent |

### 2. Lambda Features Available But Not Used

| AWS Feature | Description | Could Enable |
|---|---|---|
| **Layers** | Shared code/data packages | Reduce image size, share dependencies |
| **Versions & Aliases** | Immutable function versions + named aliases | Blue/green, canary deployments |
| **Provisioned Concurrency** | Pre-warmed execution environments | Eliminate cold starts |
| **Dead Letter Queues** | Failed invocations sent to SQS/SNS | Error handling for failed containers |
| **Event Source Mappings** | Auto-invoke from SQS, Kinesis, DynamoDB | Event-driven container execution |
| **Function URLs** | HTTP endpoints without API Gateway | Direct HTTP access to functions |
| **SnapStart** | JVM snapshot for faster starts | Java container performance |
| **Concurrency controls** | Reserved / maximum concurrency | Rate limiting |
| **Streaming response** | Response streaming for long outputs | Real-time output streaming |
| **Ephemeral storage** | Up to 10GB `/tmp` | Larger temporary storage |

### 3. CloudWatch Logs Features Available But Not Used

| AWS Feature | Description | Could Enable |
|---|---|---|
| **Log Insights** | SQL-like query language | Better log searching |
| **Metric Filters** | Extract metrics from log patterns | Auto-alerting |
| **Subscription Filters** | Stream logs to Lambda/Kinesis/Firehose | Real-time log processing |
| **Log groups with retention** | Auto-delete old logs | Storage cost management |
| **Cross-account log sharing** | Share logs across AWS accounts | Multi-tenant logging |

---

## Part D: Summary of Critical Gaps

### Docker API Gaps (things users would expect to work)

1. **`docker exec` without agent** — both ECS and Lambda require a connected
   agent for exec to work. ECS could use native ECS Exec (SSM-based) instead.

2. **`docker pause/unpause`** — no Fargate equivalent. Could be documented as
   unsupported or mapped to task stop/start (lossy).

3. **`docker attach` without agent** — same agent dependency as exec.

4. **`docker stats` fidelity** — returns synthetic data without agent. ECS
   Container Insights could provide real metrics.

5. **Signal-specific kill** — all signals map to StopTask. SIGTERM should
   ideally send a graceful shutdown signal (ECS supports 30s stop timeout).

### Cloud Feature Gaps (opportunities for improvement)

1. **ECS Exec** — would eliminate the need for the custom agent for exec
   operations. Uses AWS SSM Agent built into Fargate platform version 1.4.0+.

2. **ECS Services** — would enable long-running container support, not just
   one-shot tasks.

3. **Lambda Function URLs** — would simplify invocation without needing the
   Lambda SDK.

4. **Secrets Manager** — would provide native `docker secret`-like functionality.
