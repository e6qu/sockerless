# Lambda exec via agent-as-handler

## Problem

Lambda-backed containers in sockerless don't support `docker exec` in live mode. `ExecStart` in `backends/lambda/backend_delegates.go` delegates to `BaseServer.ExecStart`, which in simulator mode targets a host-side agent launched by the E2E harness. In live AWS, the Lambda function is the user's image booting up and running its entrypoint inside the Lambda runtime — there is no host-side process to attach to, and AWS provides no equivalent of ECS ExecuteCommand / SSM for Lambda.

For CI runner use (GitHub Actions, GitLab Runner), this is limiting: runners rely on exec to inject per-step scripts into a long-running helper container. Without exec, Lambda can only run single-shot jobs via the container's CMD.

## Design

Embed a reverse agent as a co-process inside the Lambda invocation, piggybacking on the existing `agent/cmd/sockerless-agent` binary.

### Lambda container image composition

The sockerless Lambda backend produces a layered image on top of the user's requested image:

```dockerfile
FROM <user-image>                       # e.g. node:20
COPY /opt/sockerless/sockerless-agent   /opt/sockerless/
COPY /opt/sockerless/sockerless-lambda-bootstrap /opt/sockerless/
ENTRYPOINT ["/opt/sockerless/sockerless-lambda-bootstrap"]
```

The overlay is built by `backends/lambda/image_inject.go` at `ContainerCreate` time and pushed to ECR (reusing `image_resolve.go`'s push path) before the Lambda function is created. The user's original `ENTRYPOINT` / `CMD` are captured in env vars (`SOCKERLESS_USER_ENTRYPOINT`, `SOCKERLESS_USER_CMD`) so the bootstrap can exec them.

### Bootstrap binary

`agent/cmd/sockerless-lambda-bootstrap` is the Lambda function's entrypoint. Each invocation:

1. Reads `SOCKERLESS_CALLBACK_URL`, `SOCKERLESS_CONTAINER_ID`, `SOCKERLESS_USER_ENTRYPOINT`, `SOCKERLESS_USER_CMD` from env.
2. Registers with the Lambda Runtime API (`$AWS_LAMBDA_RUNTIME_API`) — polls `/next`, returns control on `/response` or `/error`.
3. On `/next`, spawns two co-processes:
   - The user's entrypoint + cmd as a subprocess. Stdout/stderr captured to CloudWatch via Lambda's built-in plumbing.
   - `sockerless-agent -callback $CALLBACK_URL -session $CONTAINER_ID -keep-alive` connecting back to the sockerless backend. The agent opens a WebSocket and waits for multiplexed exec requests.
4. The bootstrap blocks until **either**:
   - The user's subprocess exits → bootstrap sends `response` to Runtime API with exit code, then exits.
   - The backend calls `ContainerStop` / `ContainerKill` → backend's disconnect logic closes the WebSocket → agent observes disconnect → agent returns → bootstrap SIGTERMs the subprocess → sends `response` with the stop exit code (137).
5. After `response` is sent, Lambda freezes the sandbox; on the next invocation everything re-initializes.

### Exec flow

1. Client calls `docker exec <container> <cmd>`.
2. HTTP handler reaches `Server.ExecStart` in the Lambda backend.
3. Backend looks up the reverse-agent connection registered by `SOCKERLESS_CONTAINER_ID` in a new in-process registry on `BaseServer`.
4. Sends an exec request over the WebSocket.
5. Agent (inside the Lambda container) runs the exec against the subprocess namespace via `nsenter` / `ptrace`-based attach; multiplexes stdout/stderr/stdin back over the WebSocket.
6. HTTP response returns the exec result to the Docker client.

### `ContainerStop` termination path

Builds on P86-004a. `ContainerStop` → `disconnectReverseAgent(containerID)` → registry closes the WebSocket → bootstrap returns → Lambda invocation ends. This is the only path that cuts an in-flight invocation short; `UpdateFunctionConfiguration(Timeout=1)` only protects against lingering retries.

## What this PR delivers

- `docs/LAMBDA_EXEC_DESIGN.md` — this doc.
- `agent/cmd/sockerless-lambda-bootstrap/main.go` — compile-clean skeleton with the Runtime API stub and subprocess-spawn scaffolding. Full integration with the Lambda Runtime API will land in a follow-up PR during the AWS track, since it requires a real `$AWS_LAMBDA_RUNTIME_API` endpoint to exercise.
- `backends/lambda/config.go` — `CallbackURL` field + env wiring.
- `backends/lambda/image_inject.go` — stub for building the layered image, invoked by `image_resolve.go`. Unit-testable signature.

## What stays deferred to AWS track

- End-to-end test against real Lambda: build real bootstrap, publish to ECR, invoke, exec through sockerless.
- Measurement of cold-start penalty introduced by the overlay layer.
- Bootstrap support for tty / stdin exec sessions (non-interactive first).
- Security review of the Runtime API callback path — the agent WebSocket currently has no mutual auth; a token must be configured before exposing the callback URL publicly.
- IAM policy for the Lambda execution role (needs `logs:*`, optionally VPC).

## Alternatives considered

**External sidecar** — run exec through a separate service outside Lambda. Rejected: Lambda's isolation model means the sidecar can't reach the function's namespace.

**Replace Lambda container entrypoint with agent directly** — the agent would need to know about Lambda Runtime API itself, duplicating bootstrap logic. Keeping agent generic and bootstrap Lambda-specific is cleaner.

**`docker attach` instead of `docker exec`** — attach streams stdio of the original entrypoint. Works for some CI flows but doesn't let runners inject new commands mid-job. Exec is required.
