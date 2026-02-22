# Backend Compatibility Matrix

## Current Status

| Backend   | Exec | Attach | Networks | Volumes | Agent | Runner-Compatible | Runner Tests |
|-----------|:----:|:------:|:--------:|:-------:|:-----:|:-----------------:|:------------:|
| Memory    | Y    | Y      | Y        | Y       | N     | YES               | memory (always) |
| Docker    | Y    | Y      | Y        | Y       | N     | YES               | N/A (passthrough) |
| ECS       | Y    | Y      | Y        | Y       | Y     | YES               | via `SOCKERLESS_ECS_SOCKET` |
| Cloud Run | Y    | Y      | Y        | Y       | Y     | YES               | via `SOCKERLESS_CLOUDRUN_SOCKET` |
| ACA       | Y    | Y      | Y        | Y       | Y     | YES               | via `SOCKERLESS_ACA_SOCKET` |
| Lambda    | Y†   | N      | Y        | Y       | Y†    | YES†              | via `SOCKERLESS_LAMBDA_SOCKET` |
| GCF       | Y†   | N      | Y        | Y       | Y†    | YES†              | via `SOCKERLESS_GCF_SOCKET` |
| AZF       | Y†   | N      | Y        | Y       | Y†    | YES†              | via `SOCKERLESS_AZF_SOCKET` |

†Requires: agent binary in image, `SOCKERLESS_CALLBACK_URL` configured, backend reachable from FaaS network. Subject to function timeout limits.

## Simulator Integration Tests

All cloud backends can be tested locally against simulators using `SOCKERLESS_ENDPOINT_URL`:

```bash
make sim-test-all   # all 6 backends against simulators
make sim-test-ecs   # just ECS
make sim-test-aws   # ECS + Lambda
make sim-test-gcp   # Cloud Run + GCF
make sim-test-azure # ACA + AZF
```

| Backend   | Sim Tests | PASS | Known Failures |
|-----------|:---------:|:----:|----------------|
| ECS       | 6         | 6    | — |
| Lambda    | 7         | 7    | — |
| Cloud Run | 6         | 6    | — |
| GCF       | 7         | 7    | — |
| ACA       | 6         | 6    | — |
| AZF       | 7         | 7    | — |

## Real Runner Smoke Tests

Unmodified runner binaries tested against Sockerless + simulators via Docker-based smoke tests:

| Runner | Backend | Status |
|--------|---------|:------:|
| `act` (GitHub Actions) | Memory | PASS |
| `act` (GitHub Actions) | ECS (sim) | PASS |
| `act` (GitHub Actions) | Cloud Run (sim) | PASS |
| `act` (GitHub Actions) | ACA (sim) | PASS |
| `gitlab-runner` (docker executor) | Memory | PASS |
| `gitlab-runner` (docker executor) | ECS (sim) | PASS |
| `gitlab-runner` (docker executor) | Cloud Run (sim) | PASS |
| `gitlab-runner` (docker executor) | ACA (sim) | PASS |

```bash
make smoke-test-act-all        # act against memory + all 3 simulator backends
make smoke-test-gitlab-all     # gitlab-runner against memory + all 3 simulator backends
```

## Full Terraform Integration Tests

Full terraform modules (`terraform/modules/*`) apply and destroy cleanly against local simulators. These create 5–21 resources each (VPCs, IAM, clusters, registries, storage, etc.):

| Backend   | Cloud | Resources | Apply | Destroy | Status |
|-----------|-------|:---------:|:-----:|:-------:|:------:|
| ECS       | AWS   | 21        | PASS  | PASS    | PASS   |
| Lambda    | AWS   | 5         | PASS  | PASS    | PASS   |
| CloudRun  | GCP   | 13        | PASS  | PASS    | PASS   |
| GCF       | GCP   | 7         | PASS  | PASS    | PASS   |
| ACA       | Azure | 18        | PASS  | PASS    | PASS   |
| AZF       | Azure | 11        | PASS  | PASS    | PASS   |

```bash
make tf-int-test-all    # All 6 backends (Docker, ~10-15 min)
make tf-int-test-aws    # ECS + Lambda
make tf-int-test-gcp    # CloudRun + GCF
make tf-int-test-azure  # ACA + AZF
```

## Notes

- **Memory**: In-memory backend with real WASM command execution (41 BusyBox applets + mvdan.cc/sh shell). Supports pipes, redirects, variable expansion, volumes, `docker cp`, and interactive shell. No network access from WASM sandbox.
- **Docker**: Passthrough to a real Docker daemon. All capabilities delegated.
- **ECS**: AWS Fargate tasks with agent sidecar for exec/attach. Core's enhanced exec handler dials the agent automatically.
- **Cloud Run**: GCP Cloud Run Jobs with agent injection for exec/attach. Core's enhanced exec handler dials the agent automatically.
- **ACA**: Azure Container Apps Jobs with agent injection for exec/attach. Core's enhanced exec handler dials the agent automatically.
- **Lambda**: AWS Lambda container image functions. When `SOCKERLESS_CALLBACK_URL` is set, the agent is injected into the function entrypoint and dials back to the backend via reverse WebSocket. Real exec runs inside the Lambda container.
- **GCF**: GCP Cloud Run Functions (2nd gen). When `SOCKERLESS_CALLBACK_URL` is set, the agent is injected and connects back via reverse WebSocket for real exec.
- **AZF**: Azure Functions with container image support. When `SOCKERLESS_CALLBACK_URL` is set, the agent is injected via `AppCommandLine` and connects back via reverse WebSocket for real exec.

## FaaS Agent Architecture (Reverse Connection)

Container backends (ECS, Cloud Run, ACA) use direct agent connections — the backend dials the agent at a known IP. FaaS backends can't accept inbound connections, so they use reverse WebSocket connections:

```
Container backends:  Backend ──dial ws──▶ Agent:9111 (inside container)
FaaS backends:       Agent ──dial ws──▶ Backend /internal/v1/agent/connect
```

The agent runs in "callback mode" (`--callback <url>`), connecting to the backend at startup. The backend stores the connection in an `AgentRegistry` and routes exec sessions through it with session multiplexing.

### Limitations

1. Agent binary must be present in the container image
2. Function timeout limits apply (Lambda: 15min, GCF 2nd gen: 60min, AZF consumption: 10min)
3. Attach is not supported for FaaS (main process is the function handler)
4. Backend must be network-reachable from the FaaS function via `SOCKERLESS_CALLBACK_URL`
