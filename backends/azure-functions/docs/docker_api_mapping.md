# Docker API Mapping: Azure Functions Backend

The Azure Functions backend translates Docker API calls into Azure Function App operations. A Docker "container" becomes a Function App with a custom container image, invoked via HTTP.

## Container Lifecycle

### `POST /containers/create` — Create Container

| Aspect | Vanilla Docker | AZF Backend |
|--------|---------------|-------------|
| What happens | Creates a container from an image on the local daemon | Calls `WebApps.BeginCreateOrUpdate` + `PollUntilDone` to create a Function App |
| Image | Must exist locally or be pullable | Set as `LinuxFxVersion: "DOCKER\|{imageRef}"` on the Function App |
| Entrypoint/Cmd | Stored as container config | Wrapped as `AppCommandLine` (startup command) if reverse agent enabled |
| Environment | Stored as container config | Converted to `AppSettings` (NameValuePair array) on the Function App |
| Kind | N/A | `"functionapp,linux,container"` |
| App settings | N/A | Auto-injected: `FUNCTIONS_EXTENSION_VERSION`, `WEBSITES_ENABLE_APP_SERVICE_STORAGE`, `AzureWebJobsStorage` |
| Registry | N/A | Optional `DOCKER_REGISTRY_SERVER_URL` app setting |
| Name | Container name | Function App name: `skls-{containerID[:12]}` |
| Return value | Container ID | Container ID (locally generated) |

**AZF-specific details:**
- Function App creation is synchronous (polls until done)
- Function URL and Resource ID extracted from response
- Agent env vars injected as app settings: `SOCKERLESS_AGENT_TOKEN`, `SOCKERLESS_CONTAINER_ID`, `SOCKERLESS_AGENT_CALLBACK_URL`

### `POST /containers/{id}/start` — Start Container

| Aspect | Vanilla Docker | AZF Backend |
|--------|---------------|-------------|
| What happens | Starts the container process | HTTP POST to function URL (async invocation) |
| Blocking | Returns immediately | Returns immediately; invoke runs in background goroutine |
| Agent mode | N/A | Reverse agent only — waits for callback (60s timeout) |
| Helper containers | N/A | Non-tail-dev-null commands auto-stop after 500ms |
| Exit handling | Process exits naturally | Goroutine waits for HTTP response, then 30 min for agent disconnect |
| Pre-create channel | N/A | `AgentRegistry.Prepare(id)` called BEFORE invoke |

### `POST /containers/{id}/stop` — Stop Container

| Aspect | Vanilla Docker | AZF Backend |
|--------|---------------|-------------|
| What happens | Sends SIGTERM, then SIGKILL after timeout | **No-op** — returns 204 without doing anything |
| Reason | Process can be signalled | Azure Functions run to completion |

### `POST /containers/{id}/kill` — Kill Container

| Aspect | Vanilla Docker | AZF Backend |
|--------|---------------|-------------|
| What happens | Sends specified signal to process | Disconnects reverse agent (unblocks invoke goroutine) |
| Effect | Process receives signal | Agent disconnected, container transitions to exited |

### `DELETE /containers/{id}` — Remove Container

| Aspect | Vanilla Docker | AZF Backend |
|--------|---------------|-------------|
| What happens | Removes container and its filesystem | Calls `WebApps.Delete` (best-effort), removes local state |
| Force | Kills running container first | Disconnects agent, stops container if running |

### `POST /containers/{id}/restart` — Restart Container

| Aspect | Vanilla Docker | AZF Backend |
|--------|---------------|-------------|
| What happens | Stops then starts the container | **No-op** — returns 204 (functions run to completion) |

## Exec

Handled by core via the driver chain → Agent → Synthetic.

| Aspect | Vanilla Docker | AZF Backend |
|--------|---------------|-------------|
| Execution | Runs in container namespace | Routed to reverse agent inside the Function App |
| Availability | Anytime while running | Only while function is executing and agent is connected |

## Images

### `POST /images/create` — Pull Image

| Aspect | Vanilla Docker | AZF Backend |
|--------|---------------|-------------|
| What happens | Downloads image layers from registry | Creates synthetic image (no download) |
| Image config | Full manifest + layers | Synthetic: hash of reference as ID |

### `POST /images/load` — Load Image

**Not implemented.** Returns `NotImplementedError`.

## Logs

### `GET /containers/{id}/logs` — Container Logs

| Aspect | Vanilla Docker | AZF Backend |
|--------|---------------|-------------|
| Source | Container stdout/stderr | Azure Monitor Log Analytics via `Logs.QueryWorkspace` |
| Query | N/A | KQL: `AppTraces \| where AppRoleName == "{functionAppName}" \| order by TimeGenerated asc` |
| Workspace | N/A | `SOCKERLESS_AZF_LOG_ANALYTICS_WORKSPACE` (required for logs) |
| Follow mode | Real-time streaming | **Not supported** (single snapshot fetch) |
| Timestamps | From Docker daemon | From `TimeGenerated` column (RFC3339Nano) |
| Stdout/stderr | Separate streams | All treated as stdout |
| Format | Docker multiplexed stream | Docker multiplexed stream (8-byte header per line) |
| Availability | Immediate | Depends on Application Insights ingestion delay |

## Networks, Volumes, Archive, System

All handled by core with synthetic/in-memory implementations.

| Feature | Vanilla Docker | AZF Backend |
|---------|---------------|-------------|
| Networks | Real Docker networks | In-memory tracking only |
| Volumes | Real Docker volumes | In-memory tracking only |
| Archive copy | Direct filesystem access | Via reverse agent or synthetic |
| System info | Real daemon info | Static: Driver=azure-functions, OS=Azure Functions, 2 CPUs, 4GB RAM |

## Pause/Unpause

Not explicitly overridden — falls through to core defaults which set state but have no real effect.

## CLI Command Mapping

| `docker` CLI command | Vanilla Docker | AZF Backend |
|---------------------|---------------|-------------|
| `docker create <image>` | Creates container locally | `WebApps.BeginCreateOrUpdate` (Function App) |
| `docker start <id>` | Starts local process | HTTP POST to function URL |
| `docker stop <id>` | SIGTERM + SIGKILL | **No-op** |
| `docker kill <id>` | Send signal | Disconnects reverse agent |
| `docker rm <id>` | Remove container + fs | `WebApps.Delete` |
| `docker logs <id>` | Read from daemon | Log Analytics `QueryWorkspace` |
| `docker logs -f <id>` | Stream from daemon | **Not supported** |
| `docker exec <id> <cmd>` | nsenter into container | Reverse agent relay |
| `docker cp <src> <id>:<dst>` | Write to container layer | Reverse agent relay |
| `docker pull <image>` | Download layers | Synthetic (metadata only) |
| `docker network create` | Create real network | In-memory only |
| `docker volume create` | Create real volume | In-memory only |
| `docker restart <id>` | Stop + start | **No-op** |

## Summary: What's Not Supported

| Feature | Reason |
|---------|--------|
| Container stop | Azure Functions run to completion |
| Container restart | No meaningful restart for FaaS |
| Log follow mode | Log Analytics queried as snapshot |
| Image load from tar | AZF uses container registry images |
| Real Docker networks | Functions are isolated |
| Real Docker volumes | No persistent storage |
| Forward agent | Functions cannot accept inbound connections (reverse only) |
| Stderr separation | Application Insights traces are single-stream |
| Port bindings | Functions use HTTP invoke, not TCP ports |
| Resource customization at runtime | Resources set at Function App creation time |
