# Docker API Mapping: Cloud Run Backend

The Cloud Run backend maps Docker API operations to Cloud Run Jobs and Cloud Run Services. Jobs are execution-scoped and fit one-shot containers. Services are the long-lived runner path because they can host the reverse-agent bootstrap for repeated `docker exec` calls.

The cloud APIs and Cloud Logging are the source of truth. The backend must not switch into simulator-specific semantics when `SOCKERLESS_ENDPOINT_URL` points at a local cloud-slice endpoint.

For the broader cross-cloud mapping, see [`specs/CLOUD_RESOURCE_MAPPING.md`](../../../specs/CLOUD_RESOURCE_MAPPING.md).

## Container Lifecycle

| Docker API | Cloud Run mapping |
|---|---|
| `POST /containers/create` | Records the pending Docker create request until the backend knows whether the container is a one-shot Job or a runner/keep-alive Service path. |
| `POST /containers/{id}/start` | Creates and runs a Cloud Run Job for one-shot execution, or creates/updates a Cloud Run Service revision for runner and pod-service workloads. Service starts wait for the reverse-agent bootstrap to register before returning. |
| `POST /containers/{id}/stop` | Cancels the Job execution or stops the Service-backed container path through the backend's recorded cloud result. |
| `POST /containers/{id}/kill` | Uses the same cloud stop primitive and records the Docker-visible stop result. |
| `DELETE /containers/{id}` | Deletes the sockerless-managed Job or Service resources and returns cleanup failures to the caller. |
| `GET /containers/json` / `GET /containers/{id}/json` | Queries Cloud Run, Cloud Logging, and backend cloud-state metadata rather than relying on local Docker state. |

## Exec, Attach, Archive, and Process APIs

Runner and Service-backed containers use the reverse-agent WebSocket registered by the overlay bootstrap. `docker exec`, archive, attach, process listing, and related filesystem operations go through that real agent session. If no session exists, the backend returns an explicit error.

There is no per-exec Service-URL or invoke-envelope path.

## Images

The backend preserves cloud image semantics. Image resolution fetches real registry metadata through the configured registry/cloud endpoints and uses Artifact Registry-compatible references for GCP. Pointing `SOCKERLESS_ENDPOINT_URL` at the simulator changes HTTP routing only; it does not change image names into local-only shortcuts.

`POST /images/load` is supported only when the loaded tar can be pushed into a configured registry path. Otherwise the backend returns an explicit unsupported-operation error.

## Logs

`GET /containers/{id}/logs` queries Cloud Logging for the Job, Service, or Function-backed resource and emits Docker mux frames. Follow mode polls Cloud Logging.

## Networks

Cloud Run does not expose Docker bridge networking. Runner pod materialization uses Cloud Run Service composition and the backend's cloud DNS/network drivers where applicable. VPC connector settings remain cloud egress configuration, not Docker L2 networking.

## Volumes

Workspace sharing uses the configured GCP storage backing, primarily `gcs-sync` for runner flows. In-memory tmpfs is the default for backends where Cloud Run exposes the memory-backed empty-dir primitive. Unsupported backing modes fail during configuration or startup.

## Unsupported Docker Features

| Feature | Reason |
|---|---|
| Pause/unpause | Cloud Run has no pause primitive. |
| Host networking | Cloud Run does not expose Docker host networking. |
| Docker bridge L2 semantics | Cloud Run uses Service networking and VPC egress, not a local Docker bridge. |
