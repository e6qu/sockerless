# Azure Simulator Gap Analysis

Comparing what the **ACA and Azure Functions backends actually call** against
what the **Azure simulator implements**, plus behavioral fidelity gaps.

> Scope: backend-used APIs only. Services not used by backends (VNets, NSGs,
> Private DNS, ACR, Storage, App Insights, Managed Identity, Authorization)
> are in a secondary section.

---

## 1. Container Apps Jobs — Gaps Between Backend Calls and Simulator

### 1.1 Implemented & Exercised

| Action | Backend uses | Simulator implements | Match |
|---|---|---|---|
| `BeginCreateOrUpdate` | Full job spec (config, template, containers, env, resources, tags) | Yes — stores job, returns 200 | OK |
| `BeginStart` | Starts execution, returns poller | Yes — returns 202 with Location header | OK |
| `BeginStopExecution` | By job + execution name | Yes — stops execution | OK |
| `BeginDelete` | By job name | Yes — deletes job | OK |
| `NewListByResourceGroupPager` | Lists jobs in RG (for recovery) — reads Tags, Name | Yes | OK |
| `JobsExecutionsClient.NewListPager` | Lists executions for job — reads Name, Status | Yes | OK |

### 1.2 Behavioral Fidelity Gaps

| Gap | Real Azure Behavior | Simulator Behavior | Impact |
|---|---|---|---|
| **LRO polling** | `BeginCreateOrUpdate` returns poller; `PollUntilDone` makes multiple requests to check provisioning state | Returns 200 synchronously (not actually async) | Backend calls `PollUntilDone` which resolves immediately — masks retry/timeout handling |
| **Start execution LRO** | `BeginStart` returns 202; `PollUntilDone` polls Location header URL | Returns 202 with Location; polling returns execution | OK — but verify Location URL returns correct execution |
| **Execution status enum** | `JobExecutionRunningState` enum: `Running`, `Processing`, `Succeeded`, `Failed`, `Stopped`, `Degraded` | May not use all enum values | Backend checks `Running`, `Failed`, `Degraded`, `Stopped`, `Succeeded` — all must be returned correctly |
| **Execution auto-stop** | Execution runs until container exits | Auto-stops after 3s if no agent | May cause false completions |
| **Provisioning states** | `Succeeded`, `Failed`, `InProgress`, `Canceled`, `Deleting` | May only return `Succeeded` | Backend doesn't check provisioning state directly (uses PollUntilDone) — OK |
| **Container resource validation** | CPU/memory combinations validated (e.g., 0.25/0.5Gi, 0.5/1Gi, etc.) | Accepted without validation | Masks invalid resource specs |
| **Managed environment reference** | Job must reference valid Container App Environment | Accepted without validation | Masks missing environment errors |

### 1.3 Missing Fields / Response Shape Gaps

| Field | Real Azure | Simulator | Risk |
|---|---|---|---|
| `Job.Properties.ProvisioningState` | Set during lifecycle | May not transition correctly | Low — backend uses PollUntilDone |
| `Execution.Properties.StartTime` / `EndTime` | Actual timestamps | May not be set | Low — backend doesn't read these |
| `Job.Properties.EventStreamEndpoint` | URL for log streaming | Not set | Backend uses Log Analytics instead |
| `Job.SystemData` | Created/modified timestamps and identity | Not set | Backend doesn't read |

---

## 2. Container App Environments — Gaps

### 2.1 Implemented & Exercised

The backends don't directly manage environments — they reference pre-existing
ones. The simulator implements Create/Get/Delete for Terraform testing.

### 2.2 Behavioral Gaps

| Gap | Real Azure | Simulator | Impact |
|---|---|---|---|
| **CustomDomainConfiguration** | Complex object with certificate bindings | Returns empty struct (required by azurerm provider to avoid nil deref) | OK — workaround documented |
| **PeerAuthentication** | mTLS configuration | Returns empty struct (same workaround) | OK |
| **Log Analytics integration** | Environment requires Log Analytics workspace for log routing | Accepted without validation | Masks missing log workspace errors |

---

## 3. App Service Plans — Gaps Between Backend Calls and Simulator

### 3.1 Implemented & Exercised

| Action | Backend uses | Match |
|---|---|---|
| Azure Functions backend creates plans implicitly via `WebAppsClient.BeginCreateOrUpdate` | Simulator implements App Service Plan CRUD separately | Backend doesn't call plan APIs directly — the Function App creation includes the plan reference |

### 3.2 Behavioral Gaps

| Gap | Real Azure | Simulator | Impact |
|---|---|---|---|
| **Path casing** | `serverfarms` (go-azure-sdk) vs `serverFarms` (azurestack) | Handles both | OK |
| **SKU validation** | Validates SKU tier/name combinations (Y1/Dynamic for consumption) | Accepts any SKU | Low |
| **Plan-to-app binding** | Plan must exist before function app references it | No validation | Masks ordering bugs |

---

## 4. Azure Functions (Web Apps) — Gaps Between Backend Calls and Simulator

### 4.1 Implemented & Exercised

| Action | Backend uses | Match |
|---|---|---|
| `WebAppsClient.BeginCreateOrUpdate` | Full site spec (kind, tags, SiteConfig, LinuxFxVersion, AppSettings, ServerFarmID) | OK |
| `WebAppsClient.Delete` | By name | OK |
| `WebAppsClient.NewListByResourceGroupPager` | Lists sites in RG (for recovery) — reads Tags, ID | OK |

### 4.2 Behavioral Gaps

| Gap | Real Azure | Simulator | Impact |
|---|---|---|---|
| **Function App state** | `Running`, `Stopped` — controllable via Start/Stop APIs | No Start/Stop implemented | Backend doesn't call Start/Stop — it uses create/delete lifecycle |
| **DefaultHostName** | Generated as `{appName}.azurewebsites.net` | Set from request Host header | Backend reads `DefaultHostName` to construct invocation URL — must be reachable |
| **Function invocation** | HTTP trigger at `https://{host}/api/{functionName}` with function key auth | Simulator: `POST /api/function` matching by Host header | Backend invokes via HTTP — URL must match |
| **App Settings propagation** | Settings available as env vars in function runtime | Stored but no runtime exists | OK — backend doesn't verify runtime behavior |
| **LinuxFxVersion** | Validated format: `DOCKER|{image}` | Accepted as-is | OK |
| **System-assigned identity** | Can be enabled on function app | Not supported | Backend doesn't use managed identity on function apps |
| **Deployment slots** | Staging/production slots | Not implemented | Backend doesn't use slots |

### 4.3 Missing

| Gap | Detail |
|---|---|
| **Start/Stop Function App** | Real Azure has `WebApps.Stop()` / `WebApps.Start()`. Backend doesn't use these — it creates and deletes function apps. |
| **Function keys** | Real Azure requires function keys for HTTP trigger auth. Simulator doesn't enforce. | OK for testing. |

---

## 5. Azure Monitor (Log Analytics) — Gaps Between Backend Calls and Simulator

### 5.1 Implemented & Exercised

| Action | Backend uses | Match |
|---|---|---|
| `LogsClient.QueryWorkspace` | KQL query for `ContainerAppConsoleLogs_CL` (ACA) or `AppTraces` (Functions) | OK |

### 5.2 Behavioral Gaps

| Gap | Real Azure | Simulator | Impact |
|---|---|---|---|
| **KQL parsing** | Full KQL language (aggregations, joins, functions, time ranges) | Simple parsing: `where field == 'value'` and `take N` / `limit N` | **MEDIUM** — backends send simple queries, but must handle AND clauses and comparison operators |
| **Table schema** | `ContainerAppConsoleLogs_CL` has specific column names (`TimeGenerated`, `ContainerGroupName_s`, `Log_s`, etc.) | Returns columns as defined in query response | Backend maps columns by name — must match expected schema |
| **Log ingestion latency** | Real Azure has minutes of latency between log emission and query availability | Immediate availability | Masks latency-related issues in log streaming |
| **AppTraces table** | Used by Azure Functions backend; columns include `TimeGenerated`, `Message`, `AppRoleName` | Must match expected column names | Backend reads by column name |
| **Log source** | Logs must be written to workspace by simulator's Container Apps / Functions handlers | Verify that execution/invocation handlers inject log entries | If no logs written, queries return empty |

### 5.3 Critical Concern

The backends construct KQL queries like:
```
ContainerAppConsoleLogs_CL | where ContainerGroupName_s == 'jobName' | where TimeGenerated > datetime(2024-01-01T00:00:00Z) | take 100
```

The simulator must:
1. Parse the table name from the query
2. Apply `where` clauses correctly
3. Return rows with matching column names
4. Handle `datetime()` function in comparisons

If any of these fail, log streaming will silently return empty or incorrect data.

---

## 6. OAuth2 / Authentication — Gaps

| Gap | Real Azure | Simulator | Impact |
|---|---|---|---|
| **Token validation** | Bearer tokens validated for scope, audience, expiry | Passthrough — any token accepted | OK for testing |
| **Token format** | Real JWT with RS256 signature | Mock JWT with empty signature | OK — SDK doesn't validate token internally |
| **Client credentials** | Client secret / certificate validated | Any credentials accepted | OK for testing |
| **Managed Identity** | IMDS endpoint at `169.254.169.254` for token acquisition | Not implemented (uses explicit credentials) | Backend uses `azidentity.NewDefaultAzureCredential` which tries IMDS | Low — backend runs outside Azure |

---

## 7. Secondary Services (Not Called by Backends)

### 7.1 Virtual Networks

| Area | Implemented | Real Azure Has Additionally |
|---|---|---|
| VNets | CRUD | Peering, DDoS protection, service endpoints, address space validation |
| Subnets | CRUD + delegation + NSG ref | Service endpoint policies, NAT gateway association |
| NSGs | CRUD + rules | Flow logs, application security groups, effective rules |

### 7.2 Private DNS

| Area | Implemented | Real Azure Has Additionally |
|---|---|---|
| Zones | CRUD + auto SOA | AAAA records, CNAME, MX, TXT, SRV, PTR, CAA records |
| A Records | CRUD | Multi-value records (multiple IPs per record set) |
| VNet Links | CRUD | Auto-registration of VM DNS names |

### 7.3 ACR (Container Registry)

| Area | Implemented | Real Azure Has Additionally |
|---|---|---|
| Registry | CRUD + name check | Geo-replication, webhooks, tasks, import, content trust |
| OCI Distribution | Push/pull manifests + blobs | Catalog, tag listing, referrers, ORAS artifacts |
| **Chunked upload** | Basic (POST + PUT) | Full resumable chunked uploads (PATCH) | May fail for large images |

### 7.4 Storage Accounts

| Area | Implemented | Real Azure Has Additionally |
|---|---|---|
| Accounts | CRUD + keys + service props | Encryption, network rules, private endpoints, lifecycle management |
| File Shares | CRUD via ARM + data plane | Directories, files, snapshots, soft delete, sync |
| **Blob Storage** | **Not implemented** | Full blob CRUD, containers, tiers, immutability | Not needed by backends |

### 7.5 Application Insights

| Area | Implemented | Real Azure Has Additionally |
|---|---|---|
| Components | CRUD | Continuous export, work item config, analytics |
| Query | Empty result set | Real query engine with AI-powered analytics |
| **Billing** | Mock response | Real consumption tracking | OK for testing |

### 7.6 Managed Identity

Fully implemented for CRUD. No significant gaps for Terraform testing.

### 7.7 Authorization (RBAC)

| Area | Implemented | Real Azure Has Additionally |
|---|---|---|
| Role Definitions | List with filter + Get (12 built-in roles) | 100+ built-in roles, custom roles CRUD |
| Role Assignments | CRUD at any scope | Condition-based assignments, PIM, deny assignments |
| **No enforcement** | Policies stored but not evaluated | Real RBAC denies unauthorized operations | All operations succeed |

---

## 8. Summary of Critical Gaps

Priority ordering by risk to backend correctness:

1. **HIGH**: KQL query parsing — backends send structured KQL queries with
   `where`, `datetime()`, and `take`. The simulator's simplified parser must
   handle the exact query patterns the backends produce. If parsing fails,
   log streaming silently returns empty.

2. **HIGH**: Log entry injection — **CONFIRMED GAP**: the Azure simulator's
   `containerapps.go` does NOT write log entries to Log Analytics when
   executions run (unlike the AWS simulator which auto-injects CloudWatch
   events in `ecs.go:582`). Without this, all KQL queries return empty.
   The `dataCollectionRules` endpoint exists for external ingestion but
   nothing calls it automatically.

3. **MEDIUM**: Execution status enum values — the ACA backend checks for
   specific `JobExecutionRunningState` values. The simulator must return
   the exact enum strings the SDK expects.

4. **MEDIUM**: Function App DefaultHostName — the backend reads this to
   construct the invocation URL. The simulator sets it from the request Host
   header, which may not produce a reachable URL in all deployment configs.

5. **MEDIUM**: LRO for job creation — real Azure returns a poller that requires
   multiple round-trips. Simulator returns 200 synchronously, which means
   `PollUntilDone` resolves in one call. This masks polling retry logic.

6. **LOW**: Execution auto-stop after 3s — same concern as AWS/GCP simulators.

7. **LOW**: Container resource validation — invalid CPU/memory combinations
   accepted without error.
