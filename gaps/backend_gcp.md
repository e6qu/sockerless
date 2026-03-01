# GCP Backend Gap Analysis (Cloud Run + Cloud Functions)

Comparing the backends against:
1. **Docker API coverage** — what Docker operations work end-to-end on cloud
2. **Cloud SDK feature usage** — what GCP features exist but aren't leveraged

---

## Part A: Docker API Coverage on Cloud Run Backend

### 1. Container Lifecycle

| Docker API | Cloud Run Backend | Status | Notes |
|---|---|---|---|
| `POST /containers/create` | Stores container config in local state | **FULL** | Config stored; Cloud Run Job not created until start |
| `POST /containers/{id}/start` | Calls `CreateJob` + `RunJob` | **FULL** | Creates Cloud Run Job, then runs it (creates Execution) |
| `POST /containers/{id}/stop` | Calls `CancelExecution` | **FULL** | Cancels the active execution |
| `POST /containers/{id}/kill` | Calls `CancelExecution` | **FULL** | Same as stop — no granular signal support |
| `DELETE /containers/{id}` | Calls `DeleteJob` | **FULL** | Cleans up Cloud Run Job resource |
| `POST /containers/{id}/wait` | Polls `GetExecution` for completion | **FULL** | Checks RunningCount, SucceededCount, FailedCount |
| `POST /containers/{id}/restart` | Handled by core (stop + start) | **FULL** | |
| `POST /containers/{id}/pause` | **NOT SUPPORTED** | **GAP** | Cloud Run Jobs have no pause concept |
| `POST /containers/{id}/unpause` | **NOT SUPPORTED** | **GAP** | |
| `POST /containers/{id}/update` | **NOT SUPPORTED** | **GAP** | Cannot update running execution |
| `POST /containers/{id}/rename` | Handled by core (local state only) | **PARTIAL** | Renames locally; Cloud Run Job name unchanged |

### 2. Container Inspection & Streaming

| Docker API | Cloud Run Backend | Status | Notes |
|---|---|---|---|
| `GET /containers/{id}/json` | Handled by core (local state) | **FULL** | |
| `GET /containers/json` | Handled by core | **FULL** | |
| `GET /containers/{id}/logs` | Override: reads Cloud Logging via logadmin | **FULL** | Filters by resource.type and job_name |
| `POST /containers/{id}/attach` | **Via agent only** | **PARTIAL** | Requires forward/reverse agent |
| `GET /containers/{id}/top` | Synthetic | **STUB** | |
| `GET /containers/{id}/stats` | Synthetic | **STUB** | |
| `GET /containers/{id}/changes` | Stubbed (empty) | **STUB** | |
| `GET /containers/{id}/export` | **Via agent only** | **PARTIAL** | |

### 3. Exec

| Docker API | Cloud Run Backend | Status | Notes |
|---|---|---|---|
| `POST /containers/{id}/exec` | Creates exec in store | **FULL** | |
| `POST /exec/{id}/start` | **Via agent only** | **PARTIAL** | No native exec equivalent in Cloud Run |
| `GET /exec/{id}/json` | Handled by core | **FULL** | |

### 4. Images

| Docker API | Cloud Run Backend | Status | Notes |
|---|---|---|---|
| `POST /images/create` (pull) | Override: may handle Artifact Registry auth | **FULL** | |
| All other image ops | Handled by core (synthetic) | **FULL** | |

### 5. Docker API Operations Not Possible on Cloud Run

| Feature | Why |
|---|---|
| `pause` / `unpause` | Cloud Run Jobs don't support pause |
| `update` | Cannot modify running execution |
| Signal-specific `kill` | Only cancellation; no signal granularity |
| `commit` | No filesystem snapshot |
| Privileged mode | Not supported |
| Host networking | Cloud Run manages networking |
| Device access | Not supported |
| Long-running processes | Cloud Run Jobs have max timeout (default 10min, max 24h) |

---

## Part B: Docker API Coverage on Cloud Functions Backend

### 1. Container Lifecycle

| Docker API | Cloud Functions Backend | Status | Notes |
|---|---|---|---|
| `POST /containers/create` | Calls `CreateFunction` | **FULL** | Creates Cloud Function with Docker runtime |
| `POST /containers/{id}/start` | HTTP POST to function's service URI | **FULL** | Async invocation |
| `POST /containers/{id}/stop` | No-op (function runs to completion) | **PARTIAL** | Cannot stop a running function |
| `POST /containers/{id}/kill` | Disconnects agent | **PARTIAL** | |
| `DELETE /containers/{id}` | Calls `DeleteFunction` | **FULL** | |
| `POST /containers/{id}/wait` | Handled by core | **PARTIAL** | Waits for agent disconnect |
| `POST /containers/{id}/pause` | **NOT SUPPORTED** | **GAP** | |
| `POST /containers/{id}/unpause` | **NOT SUPPORTED** | **GAP** | |

### 2. Container Inspection & Streaming

| Docker API | Cloud Functions Backend | Status | Notes |
|---|---|---|---|
| `GET /containers/{id}/logs` | Override: reads Cloud Logging (resource.type=cloud_run_revision) | **FULL** | |
| Other inspection ops | Synthetic/agent-dependent | **PARTIAL** | |

### 3. Docker API Operations Not Possible on Cloud Functions

Same as Lambda — stateless, event-driven, no persistent filesystem, no attach,
max timeout (60min for 2nd gen), no port mapping control.

---

## Part C: Cloud SDK Feature Gaps

### 1. Cloud Run Features Available But Not Used

| GCP Feature | Description | Could Enable |
|---|---|---|
| **Cloud Run Services** (not Jobs) | Long-running, auto-scaling HTTP services | Persistent container workloads (currently only uses Jobs) |
| **Cloud Run multi-container** | Sidecar containers in same pod | Multi-container deployments |
| **Startup / liveness probes** | HTTP/TCP/gRPC health checks | Docker HEALTHCHECK equivalent |
| **Cloud Run Exec** | `gcloud run jobs executions connect` (preview) | `docker exec` without custom agent |
| **Cloud SQL connections** | Built-in Cloud SQL Proxy | Database connectivity |
| **GPU support** | NVIDIA L4 GPUs on Cloud Run | GPU workloads |
| **Session affinity** | Sticky sessions for Services | Stateful containers |
| **Traffic splitting** | Percentage-based traffic routing between revisions | Canary deployments |
| **VPC egress settings** | `all-traffic` vs `private-ranges-only` | Network control |
| **Direct VPC** | Direct VPC without connector | Simpler networking |
| **Min instances** | Keep instances warm | Eliminate cold starts |
| **Cloud Run volume mounts** | GCS FUSE, NFS, in-memory volumes | Persistent storage |
| **Secret Manager integration** | Mount secrets as env vars or volumes | Secure secret management |

### 2. Cloud Functions Features Available But Not Used

| GCP Feature | Description | Could Enable |
|---|---|---|
| **Event-driven triggers** | Pub/Sub, Cloud Storage, Firestore triggers | Automatic container invocation on events |
| **Concurrency settings** | Max instances, min instances, concurrency per instance | Scale control |
| **Secret Manager** | Native secret injection | Secure environment variables |
| **2nd gen improvements** | Longer timeouts (60min), larger instances, concurrency | Better container compat |
| **Build config** | Buildpacks, Docker, custom build steps | Flexible image building |

### 3. Cloud Logging Features Available But Not Used

| GCP Feature | Description | Could Enable |
|---|---|---|
| **Structured logging** | JSON payloads with severity, labels | Richer log metadata |
| **Log sinks** | Route logs to BigQuery, Cloud Storage, Pub/Sub | Log archival and analysis |
| **Log-based metrics** | Create Cloud Monitoring metrics from log patterns | Alerting |
| **Log Analytics** | SQL-like queries over logs | Better log searching |
| **Error Reporting** | Auto-detect and group errors from logs | Error tracking |

---

## Part D: Summary of Critical Gaps

### Docker API Gaps

1. **`docker exec` without agent** — Cloud Run has an experimental `connect`
   feature (preview) that could eliminate the agent dependency.

2. **`docker pause/unpause`** — no Cloud Run equivalent. Must be documented
   as unsupported.

3. **`docker attach` without agent** — agent-dependent.

4. **`docker stats` fidelity** — synthetic data. Cloud Run provides some
   metrics via Cloud Monitoring but not per-container process-level stats.

5. **`docker stop` on Cloud Functions** — cannot stop a running function.
   The function runs until completion or timeout.

### Cloud Feature Gaps

1. **Cloud Run Services** — the backend only uses Jobs (one-shot execution).
   Services would enable long-running containers with auto-scaling, which is
   a major Docker use case.

2. **Cloud Run Exec** — preview feature that could replace the custom agent
   for interactive shell access.

3. **Cloud Run volume mounts** — GCS FUSE and NFS mounts would provide
   real persistent volumes, improving Docker volume compatibility.

4. **Secret Manager** — native secret injection would provide `docker secret`
   functionality.

5. **Cloud Run multi-container** — sidecars would enable more complex
   multi-container workloads that Docker Compose users expect.
