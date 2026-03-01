# AWS Simulator Gap Analysis

Comparing what the **ECS and Lambda backends actually call** against what the
**AWS simulator implements**, plus behavioral fidelity gaps.

> Scope: backend-used APIs only. Services not used by backends (S3, Cloud Map,
> EC2 networking, IAM, STS, EFS) are included as a secondary section since they
> exist in the simulator for Terraform/SDK testing.

---

## 1. ECS — Gaps Between Backend Calls and Simulator

### 1.1 Implemented & Exercised (no gaps)

| Action | Backend uses | Simulator implements | Match |
|---|---|---|---|
| `RegisterTaskDefinition` | Full task def with volumes, mount points, log config, tags, roles | Yes — stores full definition, auto-increments revision | OK |
| `RunTask` | Fargate launch, VPC config, tags, count=1 | Yes — async PENDING→RUNNING, agent subprocess | OK |
| `DescribeTasks` | Polls LastStatus, StoppedReason, Attachments (ENI), Containers[].ExitCode | Yes — returns all fields | OK |
| `StopTask` | cluster + taskARN + reason | Yes — sets STOPPED | OK |
| `DeregisterTaskDefinition` | taskDefARN | Yes — marks INACTIVE | OK |
| `ListTasks` | Not called by backend directly | Available for SDK tests | N/A |

### 1.2 Behavioral Fidelity Gaps

| Gap | Real AWS Behavior | Simulator Behavior | Impact |
|---|---|---|---|
| **ENI attachment details** | `DescribeTasks` returns `Attachments` with type `ElasticNetworkInterface` and `details` containing `privateIPv4Address` | Simulator uses `privateIPv4Address` (verified in `ecs.go:511`) matching backend's `eni.go:14` lookup | **OK — verified matching** |
| **Task transition timing** | PROVISIONING → PENDING → RUNNING (minutes for Fargate) | PENDING → RUNNING in 500ms | Acceptable for testing but masks timeout bugs |
| **Container exit codes** | Each container in task has its own `ExitCode` (*int32, nil while running) | Returns `ExitCode` on all containers but may not handle nil correctly for running tasks | Backend reads `Containers[].ExitCode` |
| **Task auto-stop** | Tasks run until container exits | Auto-stops after 3s unless agent manages | May cause false positives if tests are slow |
| **StoppedReason vs StopCode** | Real AWS returns `StopCode` enum (`TaskFailedToStart`, `EssentialContainerExited`, etc.) | Only returns `StoppedReason` string, no `StopCode` | Backend only reads `StoppedReason` — OK for now |
| **DescribeTasks failures array** | Returns `failures[]` with `arn`, `reason`, `detail` for missing tasks | Not verified — may return empty result instead of failures entry | Backend checks `len(result.Tasks) == 0` but doesn't inspect failures on describe |
| **Task tagging** | Tags on RunTask are persisted and returned by DescribeTasks | Tags stored on task — OK | OK |
| **Log driver validation** | AWS validates `awslogs` driver config (group must exist or be auto-created) | Auto-creates log group/stream — more lenient than AWS | Masks config errors |

### 1.3 Missing Fields / Response Shape Gaps

| Field | Real AWS | Simulator | Risk |
|---|---|---|---|
| `Task.Cpu` / `Task.Memory` | Returned on DescribeTasks (inherited from task def) | Not verified if propagated | Low — backend doesn't read these from task |
| `Task.PlatformVersion` | Returns `"1.4.0"` or `"LATEST"` for Fargate | May not be set | Low — backend ignores |
| `Task.AvailabilityZone` | Set based on subnet | Not set | Low — backend ignores |
| `Task.HealthStatus` | Per-container health status | Not implemented | Backend doesn't use health checks on ECS tasks |
| `TaskDefinition.Status` field values | `ACTIVE`, `INACTIVE`, `DELETE_IN_PROGRESS` | Returns `ACTIVE` or `INACTIVE` only | OK |

---

## 2. ECR — Gaps Between Backend Calls and Simulator

### 2.1 Implemented & Exercised

| Action | Backend uses | Match |
|---|---|---|
| `GetAuthorizationToken` | Retrieves base64 token for registry auth | OK — returns `AWS:password` token |

### 2.2 Behavioral Gaps

| Gap | Real AWS | Simulator | Impact |
|---|---|---|---|
| **Token expiry enforcement** | Tokens expire after 12h; re-auth required | Token has `ExpiresAt` set but not enforced | Low — backend gets fresh token each pull |
| **Token scope** | Scoped to specific registries | Returns single token for all repos | OK for testing |
| **Registry URL** | Returns `ProxyEndpoint` like `https://123456789012.dkr.ecr.us-east-1.amazonaws.com` | Returns endpoint based on request host | Backend uses returned endpoint — OK if format matches |

### 2.3 Not Used by Backend

The backend only calls `GetAuthorizationToken`. The simulator also implements
`CreateRepository`, `DescribeRepositories`, `DeleteRepository`, `BatchGetImage`,
`PutImage`, `BatchCheckLayerAvailability`, lifecycle policies, tags — these are
for SDK/Terraform testing, not exercised by the ECS/Lambda backends.

---

## 3. CloudWatch Logs — Gaps Between Backend Calls and Simulator

### 3.1 Implemented & Exercised

| Action | Backend uses | Match |
|---|---|---|
| `GetLogEvents` | ECS: logGroup + logStream + startFromHead + limit + nextToken | OK |
| `DescribeLogStreams` | Lambda: logGroup + orderBy + descending + limit=1 | OK |

### 3.2 Behavioral Gaps

| Gap | Real AWS | Simulator | Impact |
|---|---|---|---|
| **Log stream naming** | ECS backend expects `{streamPrefix}/{containerName}/{taskID}` | Simulator auto-creates stream with this format on RunTask | OK — but only if container has `awslogs` driver |
| **Lambda log streams** | Named `/aws/lambda/{fn}/YYYY/MM/DD/[$LATEST]hexid` | Simulator may not create log streams for Lambda functions | Lambda backend calls `DescribeLogStreams` first — may get 0 results |
| **GetLogEvents pagination** | `NextForwardToken` stays same when no new events (sentinel for follow mode) | Returns monotonic token — backend uses this for follow | Verify token stability behavior |
| **Log event ordering** | Events ordered by timestamp within stream | Simulator appends in order — OK | OK |
| **FilterLogEvents** | Not used by backend | Implemented in simulator for SDK tests | N/A |
| **Timestamp precision** | Milliseconds since epoch | Matches | OK |

### 3.3 Missing for Lambda Backend

| Gap | Detail |
|---|---|
| **Auto-created log groups for Lambda** | Real AWS creates `/aws/lambda/{functionName}` log group automatically. Simulator's Lambda handler should auto-create this group when a function is created/invoked, similar to how ECS auto-creates for `awslogs` driver. |
| **Log stream creation on invoke** | Real Lambda creates a new log stream per invocation with format `YYYY/MM/DD/[$LATEST]{hex}`. Simulator may not replicate this. |

---

## 4. Lambda — Gaps Between Backend Calls and Simulator

### 4.1 Implemented & Exercised

| Action | Backend uses | Match |
|---|---|---|
| `CreateFunction` | Full function spec (image, role, memory, timeout, env, VPC, tags) | OK |
| `Invoke` | Synchronous invocation, reads `FunctionError` | OK |
| `DeleteFunction` | By function name | OK |

### 4.2 Behavioral Gaps

| Gap | Real AWS | Simulator | Impact |
|---|---|---|---|
| **Function state transitions** | `Pending` → `Active` (takes seconds to minutes for container images) | Immediate `Active` state | Masks cold-start timing issues |
| **Invoke response payload** | Returns function output in body; `FunctionError` header for errors | Returns basic response; may not include realistic payload | Backend only checks `FunctionError` field — OK |
| **Concurrent invocations** | Subject to concurrency limits, throttling | No throttling | OK for testing |
| **Cold start simulation** | Real Lambda has cold starts (seconds) | Instant | OK |
| **UpdateFunctionConfiguration** | Implemented in simulator but not called by backend | N/A | N/A |
| **Function URL** | Not used by backend | Not implemented | N/A |

### 4.3 Missing

| Gap | Detail |
|---|---|
| **GetFunction** | Backend doesn't call it, but simulator implements it | N/A |
| **WaitUntilFunctionActive** | Backend doesn't use waiters; could be fragile if function isn't immediately Active | Simulator returns Active immediately — masks this |

---

## 5. Secondary Services (Not Called by Backends)

These services are in the simulator for Terraform/SDK testing. Gaps here affect
infrastructure provisioning tests, not backend operation.

### 5.1 EC2

| Area | Implemented | Real AWS Has Additionally |
|---|---|---|
| VPC | Create/Describe/Delete/Attributes | ModifyVpc, flow logs, DHCP options, peering |
| Subnet | Create/Describe/Delete/Modify | AZ placement validation, CIDR overlap checks |
| Security Groups | Full CRUD + rules | Rule deduplication, default SG auto-creation per VPC |
| Internet Gateway | Full CRUD + attach/detach | Only one IGW per VPC (not enforced) |
| NAT Gateway | Create/Describe/Delete | Elastic IP association validation |
| Route Tables | Full CRUD + routes + associations | Main route table auto-creation per VPC |
| Elastic IPs | Allocate/Describe/Release | Association/disassociation with instances |
| **Network Interfaces** | **Returns empty set** | Full ENI lifecycle | Low — only needed if testing ENI attachment |

### 5.2 IAM

| Area | Implemented | Real AWS Has Additionally |
|---|---|---|
| Roles | Full CRUD + inline/managed policies | Assume-role validation, session policies, permission boundaries |
| **Users** | **Not implemented** | Full user lifecycle, access keys, MFA | Not needed for ECS/Lambda |
| **Groups** | **Not implemented** | Group management | Not needed |
| **Instance Profiles** | **ListInstanceProfilesForRole returns empty** | Full lifecycle | Not needed for Fargate |

### 5.3 S3

| Area | Implemented | Real AWS Has Additionally |
|---|---|---|
| Buckets | Create/Head/List/Delete | Region constraint, versioning, lifecycle, CORS, ACLs, logging, replication |
| Objects | Put/Get/Head/Delete/List | Multipart upload, copy, SSE, storage classes, tagging, legal hold |

### 5.4 EFS

| Area | Implemented | Real AWS Has Additionally |
|---|---|---|
| File Systems | CRUD + lifecycle | Encryption, throughput mode, performance mode |
| Mount Targets | CRUD + security groups | AZ validation, DNS name generation |
| Access Points | CRUD | POSIX user enforcement |

### 5.5 Cloud Map

Fully implemented for the service discovery use case. No significant gaps
for the operations the ECS backend could use.

---

## 6. Summary of Critical Gaps

Priority ordering by risk to backend correctness:

1. **HIGH**: Lambda log stream auto-creation — Lambda backend calls `DescribeLogStreams`
   to find the latest stream, but the simulator may not create streams when
   functions are invoked. This would cause silent log loss.

2. ~~MEDIUM: ENI attachment detail key names~~ — **VERIFIED OK**. Both simulator
   (`ecs.go:511`) and backend (`eni.go:14`) use `privateIPv4Address`.

3. **MEDIUM**: `GetLogEvents` pagination token — verified in
   `cloudwatch.go:366`: the simulator always returns `"f/0"` as the
   `nextForwardToken` (static value). Real AWS returns a changing token
   that stays the same only when no new events arrive. The backend's follow
   mode in `logs.go` uses the token for polling — since it's always the same
   static value, the backend will re-fetch all events on each poll instead
   of only fetching new ones. This causes duplicate log lines in follow mode.

4. **LOW**: Task auto-stop after 3s — may cause flaky tests if agent connection
   takes longer than expected.

5. **LOW**: Missing `StopCode` on tasks — backend doesn't use it today, but
   it's part of the real API contract.
