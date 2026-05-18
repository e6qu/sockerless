# Docker API Mapping: Lambda Backend

The Lambda backend maps Docker container operations to AWS Lambda image-mode functions. A started container is an active Lambda invocation running the sockerless Lambda bootstrap and reverse agent.

For the broader cross-cloud mapping, see [`specs/CLOUD_RESOURCE_MAPPING.md`](../../../specs/CLOUD_RESOURCE_MAPPING.md) and [`docs/LAMBDA_EXEC_DESIGN.md`](../../../docs/LAMBDA_EXEC_DESIGN.md).

## Container Lifecycle

| Docker API | Lambda mapping |
|---|---|
| `POST /containers/create` | Records the pending create request and prepares the function image/configuration. The function is tagged with sockerless discovery metadata when created. |
| `POST /containers/{id}/start` | Invokes the function and waits for the overlay bootstrap to register a reverse-agent session. `SOCKERLESS_CALLBACK_URL` is required. |
| `POST /containers/{id}/stop` / `kill` | Uses the available Lambda/backend control path to end or mark the invocation; Lambda's platform limit remains authoritative. |
| `DELETE /containers/{id}` | Deletes the sockerless-managed function and propagates cleanup errors. |
| `GET /containers/json` / `GET /containers/{id}/json` | Derives state from Lambda functions, tags, invocation results, CloudWatch Logs, and the reverse-agent registry. |

## Exec, Attach, Archive, and Process APIs

`docker exec`, attach, archive, and process/file APIs require the reverse-agent session registered by `sockerless-lambda-bootstrap`. If the session is absent or the Lambda lifetime has expired, the backend returns an explicit error. There is no invoke-per-exec path.

## Images

Lambda uses real registry images, normally ECR references. Pull and inspect operations resolve real image metadata through registry/cloud APIs. `POST /images/load` is supported only when the loaded image can be pushed to an operator-configured registry path; otherwise the backend returns an explicit unsupported-operation error.

## Logs

`GET /containers/{id}/logs` reads CloudWatch Logs for the Lambda function and emits Docker mux frames.

## Networks and Volumes

Lambda VPC configuration controls egress and access to EFS. Docker peer-network semantics are not natively available in Lambda. Named volumes map to EFS access points when configured; unsupported bind or storage modes fail explicitly.

## Unsupported Docker Features

| Feature | Reason |
|---|---|
| Pause/unpause | Lambda exposes no pause primitive. |
| Host networking | Lambda does not expose Docker host networking. |
| Long-running pods beyond the Lambda timeout | The Lambda invocation duration limit is a hard platform limit. |
