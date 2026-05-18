# Docker API Mapping: Cloud Run Functions Backend

The Cloud Run Functions backend maps Docker containers to Cloud Functions Gen2 resources backed by Cloud Run Services. The function API owns the function metadata and trigger, while the underlying Service runs the sockerless overlay image and reverse-agent bootstrap.

For the broader cross-cloud mapping, see [`specs/CLOUD_RESOURCE_MAPPING.md`](../../../specs/CLOUD_RESOURCE_MAPPING.md).

## Container Lifecycle

| Docker API | GCF mapping |
|---|---|
| `POST /containers/create` | Records the pending Docker create request and prepares the function/service configuration. |
| `POST /containers/{id}/start` | Creates or reuses the function, updates the underlying Cloud Run Service to the overlay image, invokes the function URL, and waits for reverse-agent registration. |
| `POST /containers/{id}/stop` / `kill` | Uses the backend's cloud-state and reverse-agent control path; the platform invocation limit remains authoritative. |
| `DELETE /containers/{id}` | Deletes the sockerless-managed function and propagates cleanup errors. |
| `GET /containers/json` / `GET /containers/{id}/json` | Derives state from Cloud Functions, the underlying Cloud Run Service, labels/env metadata, invocation results, and reverse-agent state. |

## Exec, Attach, Archive, and Process APIs

`docker exec`, attach, archive, and process/file APIs require the reverse-agent session registered by the bootstrap. If no session exists, the backend returns an explicit error. There is no per-exec HTTP invoke envelope.

## Images

Image resolution preserves GCP cloud image references and fetches real registry metadata through the configured endpoints. The simulator's Artifact Registry surface must serve the same references as the live cloud. `POST /images/load` is supported only when the loaded image can be pushed to a configured registry path.

## Logs

`GET /containers/{id}/logs` queries Cloud Logging for the Function/underlying Service and emits Docker mux frames.

## Networks and Volumes

Multi-container runner pods materialize on the underlying Cloud Run Service where possible. Workspace sharing uses the configured GCP storage backing, primarily `gcs-sync` for runner flows. Unsupported storage or networking modes fail explicitly.

## Unsupported Docker Features

| Feature | Reason |
|---|---|
| Pause/unpause | Cloud Functions exposes no pause primitive. |
| Host networking | Cloud Functions/Cloud Run Services do not expose Docker host networking. |
| Native Docker bridge L2 semantics | The platform uses Service networking and VPC egress, not a local bridge. |
