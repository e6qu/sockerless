# Lambda reverse-agent exec design

Lambda-backed containers support `docker exec` through the same mandatory reverse-agent model used by the other FaaS-style backends. There is no invoke-per-exec path and no local simulator-only agent path for cloud backends.

## Runtime shape

The Lambda backend overlay-wraps the requested workload image before creating the function. The overlay copies the Lambda bootstrap into `/opt/sockerless`, sets it as the container entrypoint, and preserves the user's original entrypoint, argv, working directory, and environment through `SOCKERLESS_USER_*` variables.

`SOCKERLESS_CALLBACK_URL` and `SOCKERLESS_CONTAINER_ID` are required. Backend startup fails if the callback URL is missing because the Lambda backend cannot serve exec without an in-function reverse agent.

## Bootstrap

`agent/cmd/sockerless-lambda-bootstrap` is the function entrypoint. It:

- Connects to the Lambda Runtime API through `$AWS_LAMBDA_RUNTIME_API`.
- Materializes the workload environment from the preserved `SOCKERLESS_USER_*` values.
- Starts the reverse-agent WebSocket connection back to the backend.
- Runs invocation payloads through the real workload command path.
- Reports `lifetime_expired` before the Lambda deadline so later `docker exec` calls fail with the FaaS lifetime guidance error instead of silently reinvoking.
- Annotates ENOSPC failures with the shared agent helper and returns the real exit code.

## Exec flow

1. A Docker client creates and starts the Lambda-backed container.
2. The Lambda invocation starts the overlay bootstrap.
3. The bootstrap dials `SOCKERLESS_CALLBACK_URL` and registers under `SOCKERLESS_CONTAINER_ID`.
4. `docker exec` reaches the Lambda backend's reverse-agent exec driver.
5. The backend sends the exec request over the registered WebSocket.
6. The bootstrap runs the command inside the Lambda workload process environment and returns stdout, stderr, stdin handling, working directory, environment, and exit code through the exec envelope.

If the reverse agent is not connected by the configured bootstrap timeout, `ContainerStart` fails loudly. Later exec/archive operations also fail loudly when no agent session exists.

## Stop and lifetime

Lambda's maximum invocation duration is a hard platform limit. Sockerless does not checkpoint, reinvoke, or transparently migrate the pod. When the bootstrap reports `lifetime_expired`, the backend marks the container and subsequent exec attempts return the operator-guidance error.

`ContainerStop` and `ContainerRemove` clean up the cloud function and any created cloud resources. Cleanup failures propagate to Docker clients.

## Validation

Current coverage includes:

- Lambda backend simulator integration tests for container lifecycle and exec.
- Shared FaaS smoke coverage through `make backends/lambda/test-faas-smoke`.
- Aggregate FaaS smoke coverage through `make faas-smoke-test-all`, which CI runs after backend package tests.

Live-cloud validation remains tracked separately under BUG-1075 and must be closed only with evidence from real AWS credentials and cloud resources.
