# Docker API Mapping: ACA Backend

The Azure Container Apps backend maps Docker containers to Azure Container Apps Jobs or Apps. Jobs are execution-scoped. Apps are the runner path because they can host the sockerless bootstrap and reverse-agent session for repeated `docker exec` calls.

For the broader cross-cloud mapping, see [`specs/CLOUD_RESOURCE_MAPPING.md`](../../../specs/CLOUD_RESOURCE_MAPPING.md).

## Container Lifecycle

| Docker API | ACA mapping |
|---|---|
| `POST /containers/create` | Records the pending Docker create request until the backend chooses the Job or App materialization path. |
| `POST /containers/{id}/start` | Creates/starts an ACA Job for one-shot execution or creates/updates an ACA App revision for runner workloads. App starts wait for the reverse-agent bootstrap to register. |
| `POST /containers/{id}/stop` / `kill` | Stops the Job execution or App-backed container path and records the Docker-visible result. |
| `DELETE /containers/{id}` | Deletes sockerless-managed Job/App resources and returns cleanup failures to the caller. |
| `GET /containers/json` / `GET /containers/{id}/json` | Derives state from ARM resources, executions/replicas, tags, Log Analytics, and reverse-agent state. |

## Exec, Attach, Archive, and Process APIs

ACA Apps use the reverse-agent WebSocket registered by the overlay bootstrap. `docker exec`, attach, archive, process listing, and filesystem APIs go through that real session. If no session exists, the backend returns an explicit error.

ACA Jobs remain available for one-shot execution, but they are not the fallback path for runner exec.

## Images

The backend resolves real image metadata and uses ACR/registry references. Overlay images are built through the configured Azure registry/build path when Apps need the bootstrap. `POST /images/load` is supported only when the loaded tar can be pushed to a configured registry path.

## Logs

`GET /containers/{id}/logs` queries Azure Monitor Log Analytics for ACA Job or App rows and emits Docker mux frames.

## Networks and Volumes

ACA managed environments and private DNS provide the cloud networking surface. Workspace sharing uses Azure Files through the shared Azure volume driver. Unsupported Docker bridge, host-network, or storage modes fail explicitly.

## Unsupported Docker Features

| Feature | Reason |
|---|---|
| Pause/unpause | Azure Container Apps exposes no pause primitive. |
| Host networking | ACA does not expose Docker host networking. |
| Docker bridge L2 semantics | ACA uses managed-environment networking, not a local Docker bridge. |
