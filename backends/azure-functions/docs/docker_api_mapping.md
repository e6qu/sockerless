# Docker API Mapping: Azure Functions Backend

The Azure Functions backend maps Docker containers to Linux Function Apps running custom container images. The sockerless AZF bootstrap starts inside the Function App container and registers a reverse-agent session for Docker exec-style operations.

For the broader cross-cloud mapping, see [`specs/CLOUD_RESOURCE_MAPPING.md`](../../../specs/CLOUD_RESOURCE_MAPPING.md).

## Container Lifecycle

| Docker API | AZF mapping |
|---|---|
| `POST /containers/create` | Records the pending Docker create request and prepares Function App configuration, including app settings, image reference, storage, and overlay metadata. |
| `POST /containers/{id}/start` | Creates/updates the Function App as needed, invokes the HTTP-triggered bootstrap, and waits for reverse-agent registration. |
| `POST /containers/{id}/stop` / `kill` | Uses the backend's Function App and reverse-agent control path; the platform invocation limit remains authoritative. |
| `DELETE /containers/{id}` | Deletes sockerless-managed Function App resources and returns cleanup failures to the caller. |
| `GET /containers/json` / `GET /containers/{id}/json` | Derives state from Azure Web Apps/Function App metadata, tags/app settings, invocation results, Azure Monitor logs, and reverse-agent state. |

## Exec, Attach, Archive, and Process APIs

`docker exec`, attach, archive, and process/file APIs require the reverse-agent session registered by `sockerless-azf-bootstrap`. If no session exists, the backend returns an explicit error. There is no per-exec HTTP invoke envelope.

## Images

The backend uses real registry image references and ACR-backed overlay images. Pull/inspect operations resolve real image metadata through registry/cloud APIs. `POST /images/load` is supported only when the loaded image can be pushed to a configured registry path.

## Logs

`GET /containers/{id}/logs` queries Azure Monitor / Application Insights for Function App traces and emits Docker mux frames.

## Networks and Volumes

Azure Functions does not expose Docker bridge networking or a native multi-container sidecar primitive through this backend. Workspace sharing uses Azure Files through the shared Azure volume driver. Multi-container pods are rejected clearly; Azure workloads that require sidecars sharing `localhost` should use the ACA backend.

## Unsupported Docker Features

| Feature | Reason |
|---|---|
| Pause/unpause | Azure Functions exposes no pause primitive. |
| Host networking | Function Apps do not expose Docker host networking. |
| Native Docker bridge L2 semantics | The platform does not expose a local Docker bridge. |
| Multi-container pods / sidecars | The AZF backend manages Azure Functions, which expose one custom-container slot here. Use ACA Apps for Azure multi-container pod workloads. |
| Long-running pods beyond the platform timeout | The Function App invocation/runtime limits are hard platform limits. |
