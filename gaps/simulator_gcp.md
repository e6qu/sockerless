# GCP Simulator Gap Analysis

Comparing what the **Cloud Run and Cloud Functions backends actually call**
against what the **GCP simulator implements**, plus behavioral fidelity gaps.

> Scope: backend-used APIs only. Services not used by backends (Compute, DNS,
> GCS, IAM, Service Usage, VPC Access, Artifact Registry) are in a secondary
> section.

---

## 1. Cloud Run Jobs v2 — Gaps Between Backend Calls and Simulator

### 1.1 Implemented & Exercised (no gaps)

| Action | Backend uses | Simulator implements | Match |
|---|---|---|---|
| `CreateJob` | Full Job proto (labels, execution template, containers, env, resources, VPC, ports) | Yes — stores job, returns LRO | OK |
| `RunJob` | By job name → creates execution | Yes — creates execution, spawns agent | OK |
| `GetExecution` | Reads RunningCount, FailedCount, CancelledCount, SucceededCount, CompletionTime | Yes — returns execution with status fields | OK |
| `CancelExecution` | By execution name | Yes — sets cancelled status | OK |
| `DeleteJob` | By job name, returns LRO | Yes | OK |
| `ListJobs` | By parent (for recovery scan) — reads Labels, Name | Yes | OK |

### 1.2 Behavioral Fidelity Gaps

| Gap | Real GCP Behavior | Simulator Behavior | Impact |
|---|---|---|---|
| **LRO resolution** | CreateJob/DeleteJob return long-running operations that take seconds to complete; client calls `op.Wait()` | LRO resolves immediately (operation stored as DONE) | Masks timeout handling bugs in backend |
| **Execution status conditions** | Uses `conditions[]` with `type`, `state`, `reason` fields (e.g., `type=Completed`, `state=CONDITION_SUCCEEDED`) | Returns conditions but may not match exact real condition types | Backend reads count fields, not conditions — low impact |
| **Execution count fields** | `RunningCount`, `FailedCount`, `SucceededCount`, `CancelledCount` are task-level counts (a job can have multiple tasks with `TaskCount > 1`) | Simulator always uses TaskCount=1 and sets counts as 0 or 1 | Backend uses TaskCount=1 — OK |
| **CompletionTime** | Set to actual completion timestamp | Set when execution completes (auto-stop after 3s or agent finish) | OK |
| **Execution auto-completion** | Execution runs until container exits | Auto-completes after 3s if no agent | May cause false completions in slow tests |
| **Job revision / generation** | `Job.Generation` increments on updates | Not tracked | Backend doesn't read generation |
| **Etag / reconciling** | Jobs have `etag` and `reconciling` fields during updates | Not returned | Backend doesn't use these |
| **VpcAccess on Job** | `VpcAccess.Connector` validated against real VPC connectors | Accepted without validation | Masks config errors |
| **Resource limits** | `cpu` and `memory` validated (e.g., `"1"` CPU, `"512Mi"` memory) | Accepted without validation | Masks invalid resource specs |

### 1.3 Missing Fields / Response Shape Gaps

| Field | Real GCP | Simulator | Risk |
|---|---|---|---|
| `Job.Uri` | HTTPS URL for the job | May not be set | Backend doesn't read this |
| `Job.Creator` / `Job.LastModifier` | Email of creator | Not set | Backend doesn't read |
| `Execution.LaunchStage` | `GA`, `BETA`, etc. | Not set | Backend doesn't read |
| `Job.TerminalCondition` | Shows latest terminal condition | May not be set | Backend doesn't read |
| `Execution.LogUri` | Link to Cloud Console logs | Not set | Backend doesn't read |

---

## 2. Cloud Functions v2 — Gaps Between Backend Calls and Simulator

### 2.1 Implemented & Exercised

| Action | Backend uses | Match |
|---|---|---|
| `CreateFunction` | Full function spec (name, labels, BuildConfig, ServiceConfig) | OK |
| `DeleteFunction` | By name, returns LRO | OK |
| `ListFunctions` | By parent (for recovery) — reads Labels, Name | OK |

### 2.2 Behavioral Gaps

| Gap | Real GCP Behavior | Simulator Behavior | Impact |
|---|---|---|---|
| **Build step** | CreateFunction triggers a Cloud Build that builds the container image (minutes) | Immediate — no build step | Masks build failures |
| **ServiceConfig.Uri** | Set after deployment completes — HTTPS URL | Set from request host | Backend reads Uri for HTTP invocation — must be reachable |
| **Function state** | `ACTIVE`, `FAILED`, `DEPLOYING`, `DELETING`, `UNKNOWN` | Always `ACTIVE` after create | Masks deployment failures |
| **Invocation endpoint** | Real GCP: `POST https://{region}-{project}.cloudfunctions.net/{functionName}` | Simulator: `POST /v2-functions-invoke/{functionID}` (custom path) | Backend must be configured with simulator's invoke URL — OK |
| **Runtime validation** | Validates `docker` runtime and entry point | Accepts any runtime | Low |
| **ServiceConfig fields** | `AvailableMemory`, `AvailableCpu`, `TimeoutSeconds`, `MaxInstanceCount`, etc. validated | Stored but not validated | Low |
| **2nd gen / Cloud Run backing** | Cloud Functions v2 is backed by Cloud Run services internally | No Cloud Run service created | Backend doesn't observe the backing service |

### 2.3 Missing

| Gap | Detail |
|---|---|
| **GetFunction** | Backend doesn't call it (only creates and deletes). Simulator implements it for SDK tests. |
| **UpdateFunction** | Not implemented in simulator. Backend doesn't call it. |
| **Function invocation auth** | Real GCP requires IAM `cloudfunctions.functions.invoke` permission or `--allow-unauthenticated`. Simulator has no auth on invoke. |

---

## 3. Cloud Logging — Gaps Between Backend Calls and Simulator

### 3.1 Implemented & Exercised

| Action | Backend uses | Match |
|---|---|---|
| `Entries` (via logadmin) | Filter by resource.type, resource.labels, timestamp | OK |

### 3.2 Behavioral Gaps

| Gap | Real GCP Behavior | Simulator Behavior | Impact |
|---|---|---|---|
| **Filter syntax** | Full GCP filter language: `resource.type=`, `AND`, `OR`, `>=`, field paths, regex | Substring matching only | **HIGH** — backend sends `resource.type="cloud_run_job" AND resource.labels.job_name="X" AND timestamp>"T"` — simulator must parse AND conditions and field=value pairs |
| **Log entry structure** | `Payload` can be `TextPayload`, `JsonPayload`, or `ProtoPayload` | Stores as generic payload | Backend casts `entry.Payload` to string — works only for TextPayload |
| **resource.type values** | `cloud_run_job` for Cloud Run, `cloud_run_revision` for Cloud Functions | Stored as provided by WriteEntries | OK if entries are written with correct resource type |
| **Log entry source** | Logs come from actual container stdout/stderr | **CONFIRMED GAP**: Cloud Run Jobs handler (`cloudrunjobs.go`) does NOT write log entries when executions run. The `entries:write` endpoint exists but is only called externally. | Backend log queries will always return empty unless something externally writes entries |
| **Pagination** | `pageSize` + `pageToken` cursor-based | Implemented with simple offset | OK for small result sets |
| **Ordering** | Default newest-first; `orderBy` param available | May not respect ordering | Backend iterates all entries — OK if order doesn't matter |

### 3.3 Critical Concern

The **filter parsing** is a **confirmed critical gap**. Verified in
`simulators/gcp/logging.go:161-169`: the `filterMatch` function does simple
`strings.Contains` on `logName`, `severity`, and `textPayload` fields. It does
**not** parse structured GCP filter syntax like
`resource.type="cloud_run_job" AND resource.labels.job_name="X"`.

This means the backend's filter will be treated as a single substring, which
may accidentally match entries that contain the filter text somewhere in their
fields. In practice this may work for simple cases (the job name appears in
the logName), but it's not correct behavior and would fail for:
- Filtering by `resource.type` (the entry has no `resource.type` field to match)
- Timestamp comparisons (`timestamp>"2024-..."` is not a substring match)
- Multiple AND conditions

---

## 4. Secondary Services (Not Called by Backends)

### 4.1 Compute Engine

| Area | Implemented | Real GCP Has Additionally |
|---|---|---|
| Networks | Create/List/Get/Delete/Patch | Peering, firewall rules, routes, shared VPC |
| Subnetworks | Create/Get/Delete | Secondary ranges, private Google access, flow logs |
| **Firewall Rules** | **Not implemented** | Full CRUD | Needed for comprehensive VPC testing |
| **Routes** | **Not implemented** | Custom routes | May be needed for Terraform |
| Operations | Get (always DONE) | Real polling with status transitions | May mask async issues |

### 4.2 Cloud DNS

| Area | Implemented | Real GCP Has Additionally |
|---|---|---|
| Managed Zones | Create/List/Get/Delete | DNSSEC, forwarding, peering, response policies |
| Record Sets | Create/List/Delete | Update (PATCH), changes API, SOA/NS auto-management |
| **Changes API** | **Not implemented** | `POST /changes` for atomic record updates | May be needed by some Terraform resources |

### 4.3 Cloud Storage (GCS)

| Area | Implemented | Real GCP Has Additionally |
|---|---|---|
| Buckets | Create/List/Get/Delete | Lifecycle, versioning, retention, CORS, logging, IAM |
| Objects | Upload/Download/List/Get/Delete | Multipart upload (resumable), compose, copy, rewrite, ACLs, customer-managed encryption |
| **Resumable uploads** | **Not implemented** | Large file uploads use resumable protocol | May be needed for large artifact storage |
| **Object versioning** | **Not implemented** | Version history, generation numbers | Low priority |
| **Notifications** | **Not implemented** | Pub/Sub notifications on object changes | Not used |

### 4.4 Artifact Registry

| Area | Implemented | Real GCP Has Additionally |
|---|---|---|
| Repositories | CRUD + IAM | Tags, cleanup policies, CMEK, vulnerability scanning |
| Docker Images | List + OCI Distribution API | Image deletion via API (only via manifest delete) |
| OCI Distribution | Push/pull manifests and blobs | Catalog endpoint, tag listing, cross-repo mount |
| **Resumable blob uploads** | **Partial** (single PUT) | Chunked/resumable uploads | May fail for large images |

### 4.5 IAM

| Area | Implemented | Real GCP Has Additionally |
|---|---|---|
| Service Accounts | CRUD | Keys, signing, impersonation, workload identity |
| IAM Policies | Get/Set at project and resource level | Conditional bindings, audit config, deny policies |
| **No RBAC enforcement** | Policies stored but not evaluated | Real IAM denies unauthorized requests | All requests succeed regardless of policy |

### 4.6 VPC Access

Fully implemented for connector CRUD. No significant gaps for Terraform testing.

### 4.7 Service Usage

| Area | Implemented | Real GCP Has Additionally |
|---|---|---|
| Enable/Disable/Get/List | Yes — but always returns ENABLED optimistically | Real API tracks actual service state, dependencies | Masks "API not enabled" errors |
| **Batch Enable** | Implemented | OK | OK |

---

## 5. Summary of Critical Gaps

Priority ordering by risk to backend correctness:

1. **HIGH**: Cloud Logging filter parsing — backend sends structured AND filters
   with field paths (`resource.type`, `resource.labels.job_name`, `timestamp>`).
   If the simulator only does substring matching, log retrieval will be
   incorrect. Must verify filter parsing fidelity.

2. **HIGH**: Log entry injection — the simulator must create log entries when
   Cloud Run executions and Cloud Functions invocations run. If no entries are
   written, the backend's log streaming will always return empty.

3. **MEDIUM**: LRO immediate resolution — all long-running operations return
   DONE immediately. The backends call `op.Wait()` which will succeed instantly.
   This masks timeout handling and retry logic issues.

4. **MEDIUM**: Cloud Functions invoke URL — the simulator uses a custom path
   (`/v2-functions-invoke/{id}`) that differs from real GCP. The backend must
   be configured to use this path. If the backend constructs the URL from
   `ServiceConfig.Uri`, the simulator must return a reachable URL.

5. **LOW**: Execution auto-completion after 3s — same concern as AWS simulator.

6. **LOW**: Operations always DONE — Compute operations return DONE immediately,
   masking async provisioning issues in Terraform tests.
