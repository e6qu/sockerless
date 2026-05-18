# Docker API Mapping: ECS Backend

The ECS backend maps Docker container operations to AWS ECS/Fargate and related AWS APIs. The cloud APIs are the source of truth; the backend must not invent container state when ECS, CloudWatch, EFS, Cloud Map, or ECR can answer it.

For the broader cross-cloud mapping, see [`specs/CLOUD_RESOURCE_MAPPING.md`](../../../specs/CLOUD_RESOURCE_MAPPING.md).

## Container Lifecycle

| Docker API | ECS mapping |
|---|---|
| `POST /containers/create` | Records the pending Docker create request until start. Cloud resources are created from the Docker config when the container is started. |
| `POST /containers/{id}/start` | Registers or updates the ECS task definition as needed, then calls `RunTask` on Fargate with configured subnets, security groups, task/execution roles, logging, mounts, env, user, working directory, entrypoint, command, ports, and resource settings. |
| `POST /containers/{id}/stop` | Calls `StopTask`. Docker wait/inspect state is derived from `DescribeTasks` and container exit status. |
| `POST /containers/{id}/kill` | Uses the ECS stop primitive. Signal-specific Docker semantics are represented through the backend's stop result where ECS exposes enough state. |
| `DELETE /containers/{id}` | Stops the task when needed, deregisters sockerless-managed task definitions when appropriate, and returns cloud cleanup errors to the caller. |
| `GET /containers/json` / `GET /containers/{id}/json` | Queries ECS and related AWS APIs through the backend cloud-state provider. |

## Exec, Attach, Archive, and Process APIs

`docker exec` uses ECS ExecuteCommand over SSM Session Manager. The backend waits for the managed agent to be running before starting the session and bridges the stream back to Docker's attach protocol.

Filesystem archive operations, process listing, stats, attach, and related APIs must be served by real cloud/agent data. If the required ECS or in-task agent path is unavailable, the backend returns an explicit error.

## Images

The ECS backend uses registry images. Pull and inspect operations resolve real registry metadata through the registry APIs and cloud credentials; they do not create synthetic image configs. `POST /images/load` is supported only when the backend can push the loaded image into an operator-configured registry path. Otherwise it returns an explicit unsupported-operation error.

## Logs

`GET /containers/{id}/logs` reads CloudWatch Logs for the ECS task's configured log group and stream prefix. Follow mode polls CloudWatch and emits Docker mux frames.

## Networks

Fargate tasks always run with `awsvpc`. Docker network operations map to AWS network primitives such as security groups and Cloud Map/private DNS where configured. The task ENI address is authoritative after the task starts.

## Volumes

Docker named volumes and bind-style workspace mounts map to EFS when configured. Volume operations must operate on the real cloud backing store or fail explicitly when the backend lacks a configured storage primitive.

## Unsupported Docker Features

| Feature | Reason |
|---|---|
| Pause/unpause | ECS Fargate does not expose a pause primitive. |
| Host networking | Fargate requires `awsvpc`. |
| Docker-layer filesystem mutation without an agent/storage path | ECS does not expose a Docker daemon filesystem API. |
