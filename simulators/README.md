# Simulators

Local reimplementations of cloud-provider APIs that the Sockerless cloud backends consume. Not mocks — jobs run, functions execute, timeouts fire, logs are produced, all driven by the same configuration knobs (replica timeouts, task timeouts, etc.) that the real cloud services honor. Code that works against the simulators works against the real cloud and vice versa.

This file is the **end-to-end showcase + navigation hub**. The canonical per-cloud documentation lives in the three sub-directories — read those for full per-service detail.

| Cloud | README | Backends it serves |
|---|---|---|
| AWS | [`simulators/aws/README.md`](aws/README.md) | [`backends/ecs`](../backends/ecs/README.md), [`backends/lambda`](../backends/lambda/README.md) |
| GCP | [`simulators/gcp/README.md`](gcp/README.md) | [`backends/cloudrun`](../backends/cloudrun/README.md), [`backends/cloudrun-functions`](../backends/cloudrun-functions/README.md) |
| Azure | [`simulators/azure/README.md`](azure/README.md) | [`backends/aca`](../backends/aca/README.md), [`backends/azure-functions`](../backends/azure-functions/README.md) |

## Reference adaptors

Every simulator is paired with the same three external tools per cloud — the SDK, the official CLI, and the Terraform provider. **Anything any of these does against the real cloud's endpoint, it must do against the simulator on the same wire.** The per-sim READMEs list the exact versions + spec links.

| Cloud | SDK | CLI | Terraform provider |
|---|---|---|---|
| AWS | [`aws-sdk-go-v2`](https://github.com/aws/aws-sdk-go-v2) | [`aws`](https://docs.aws.amazon.com/cli/latest/reference/) | [`hashicorp/aws`](https://registry.terraform.io/providers/hashicorp/aws/latest/docs) |
| GCP | [`cloud.google.com/go`](https://pkg.go.dev/cloud.google.com/go) | [`gcloud`](https://cloud.google.com/sdk/docs/install) | [`hashicorp/google`](https://registry.terraform.io/providers/hashicorp/google/latest/docs) |
| Azure | [`azure-sdk-for-go`](https://pkg.go.dev/github.com/Azure/azure-sdk-for-go/sdk) | [`az`](https://learn.microsoft.com/en-us/cli/azure/install-azure-cli) | [`hashicorp/azurerm`](https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs) |

Discipline patterns for wire fidelity:

- Capturing wire shape from the SDK serializer source code — [`.claude/skills/sim-handler-checklist`](../.claude/skills/sim-handler-checklist/SKILL.md).
- Cross-resource invariants under one `terraform apply` — [`.claude/skills/cross-resource-stack-test`](../.claude/skills/cross-resource-stack-test/SKILL.md).
- The broader wire-fidelity discipline — [`.claude/skills/adaptor-fidelity-check`](../.claude/skills/adaptor-fidelity-check/SKILL.md).

## Three governing principles

1. **The simulator is a cloud slice.** `simulators/aws/` implements whatever slice of AWS sockerless depends on — ECS + ECR + Lambda + CloudWatch + Cloud Map + EC2 + STS + IAM + S3 + EFS + KMS + SSM + Secrets Manager + DynamoDB + CloudFront + ACM + Route 53 + WAFv2 + Amplify — at cloud-API fidelity. Not a per-product simulator; a cloud slice.
2. **One binary per cloud.** Adding a new service slice means a new `registerX(srv)` + handler file inside `simulators/aws/`, `simulators/gcp/`, or `simulators/azure/`. Never a new binary per product.
3. **Cloud-API fidelity.** Match the real cloud's error shapes, response headers, async operation semantics, path templates, HTTP status codes, and wire encodings exactly. When the cloud's contract doesn't cover something, neither does the simulator.

Full statement and rationale in [AGENTS.md → Simulator architecture — cloud-slice principle](../AGENTS.md#simulator-architecture--cloud-slice-principle). Enforced per-commit by the `simulator-testing-contract` pre-commit hook (every handler change must touch sdk-tests + cli-tests + terraform-tests).

## Default ports

| Simulator | HTTP | gRPC (where used) |
|---|---|---|
| AWS | `:4566` | n/a |
| GCP | `:4567` | `:4568` (Cloud Logging) |
| Azure | `:4568` | n/a |

## Quick start — all three at once

```sh
cd simulators
docker compose up
```

This starts all three simulators on their default ports with health checks. Each one logs to stdout in the same compose process.

Equivalent without compose:

```sh
cd simulators/aws && go run . &
cd simulators/gcp && go run . &
cd simulators/azure && go run . &
```

Environment knobs (per sim — full list in each sub-README):

| Variable | Default | Description |
|---|---|---|
| `SIM_LISTEN_ADDR` | `:8443` (overridden per provider) | Listen address |
| `SIM_AWS_PORT` / `SIM_GCP_PORT` / `SIM_AZURE_PORT` | `4566` / `4567` / `4568` | Provider-specific port override |
| `SIM_TLS_CERT`, `SIM_TLS_KEY` | unset | Enable HTTPS (required by some Terraform providers — see [`simulators/azure/README.md § Special handling`](azure/README.md)) |
| `SIM_LOG_LEVEL` | `info` | Log level (`trace`, `debug`, `info`, `warn`, `error`) |

## End-to-end showcase

The canonical multi-cloud workflow combines simulators across all three clouds in one CI run:

```sh
# 1. Start all three sims
cd simulators && docker compose up -d

# 2. Drive each one with its real reference adaptor
export AWS_ENDPOINT_URL=http://localhost:4566 AWS_REGION=us-east-1 \
       AWS_ACCESS_KEY_ID=test AWS_SECRET_ACCESS_KEY=test
aws ecs list-clusters
aws cloudfront list-distributions

export CLOUDSDK_API_ENDPOINT_OVERRIDES_RUN=http://localhost:4567/
gcloud run jobs list --region us-central1

az rest --method GET --url "http://localhost:4568/subscriptions/.../resourceGroups?api-version=2021-04-01"

# 3. Apply Terraform that touches all three clouds at once
cd simulators/aws/terraform-tests   && go test -run TestStackProductionShape
cd ../../gcp/terraform-tests        && go test ./...
cd ../../azure/terraform-tests      && go test ./...

# 4. Or run any sockerless backend against its sim
DOCKER_HOST=tcp://localhost:3375 docker run --rm alpine echo hello
```

Per-sim captured-output samples live in each sub-README. For the most exercised production-shape integration test, see [`simulators/aws/terraform-tests/TestStackProductionShape`](aws/terraform-tests/apply_test.go) — it provisions CloudFront + ACM + WAFv2 + Route 53 ALIAS + Amplify + IAM SLR/OIDC + ECS + Cloud Map in one `terraform apply` and asserts the cross-resource references resolve correctly.

## Validation

Every simulator ships four test surfaces:

- `sdk-tests/` — official cloud SDK against the running sim.
- `cli-tests/` — official cloud CLI against the running sim.
- `terraform-tests/` — real Terraform provider with `endpoints {}` overrides against the running sim.
- `bash-tests/` (where present) — standalone bash scripts validating CLI behaviour in text + JSON modes.

```sh
# Per-cloud run-all
cd simulators/aws/sdk-tests       && GOWORK=off go test -v ./...
cd simulators/aws/cli-tests       && GOWORK=off go test -v ./...
cd simulators/aws/terraform-tests && GOWORK=off go test -v ./...
cd simulators/aws/bash-tests      && ./test_aws_cli.sh
```

Top-level Makefile entry points:

```sh
make docker-test          # Docker-based tests for all clouds
make sim-test-all         # Simulator-backend integration tests
```

CI runs all four on every PR — see `.github/workflows/ci.yml` `sim (aws)`, `sim (gcp)`, `sim (azure)` jobs.

### Test counts (approximate)

| Cloud | SDK tests | CLI tests | Bash tests | Terraform tests |
|---|---|---|---|---|
| AWS | 46 | 26 | 61 | `TestStackProductionShape` ≈ 90s end-to-end |
| GCP | 36 | 21 | 33 | ≈ 5s apply/destroy |
| Azure | 48 | 19 | 42 | ≈ 1s apply/destroy (Docker-only, TLS) |

## Shared framework

The `shared/` directory (vendored into each simulator as a Go-module `replace`) provides:

- **server.go** — HTTP server with health check, graceful shutdown, optional TLS.
- **middleware.go** — request ID, structured logging, auth passthrough (extracts identity from SigV4 / Bearer tokens).
- **router.go** — provider-specific routing: `AWSRouter` (X-Amz-Target header), `AWSQueryRouter` (Action parameter), `GCPRouter`/`AzureRouter` (path-based).
- **state.go** — generic `StateStore[T]` with thread-safe CRUD operations.
- **errors.go** — error-response formatting per provider (AWS JSON, EC2 XML, S3 XML, GCP JSON, Azure JSON).
- **config.go** — environment-variable configuration loading.

## Design philosophy

Simulators are **real implementations**, not fakes. They don't approximate cloud behavior with synthetic timers or hardcoded responses — they reimplement the actual service semantics:

- **Execution lifecycle** is driven by cloud-native configuration. Azure ACA jobs respect `replicaTimeout`. GCP Cloud Run jobs respect the task-template `timeout`. AWS ECS tasks run until the process exits or `StopTask` is called, because ECS has no native execution timeout.
- **Log injection** writes entries to the same tables and log groups that the real services would, queryable through the same APIs (KQL for Azure, Cloud Logging filters for GCP, CloudWatch for AWS).
- **Agent integration** spawns real subprocesses — the same `sockerless-agent` binary used in production — enabling full exec/attach through simulated cloud resources.
- **SDK + Terraform compatibility** is tested with the real official clients, not custom HTTP calls.

The simulators run locally on a single machine today. The architecture is designed so they can eventually run distributed across multiple machines, with the same API surface.

## Workload execution — host model

Every execution-service (ECS, Lambda, Cloud Run, Cloud Functions, Cloud Run Jobs, ACA, App Service / AZF) runs the workload on a **Docker host** shaped per cloud-product. Workloads never run as `os/exec` host processes of the simulator binary itself — that distinction is enforced by `simulators/<cloud>/sdk-tests/host_dispatch_test.go`. The workload's `Architecture` field (default `linux/arm64`) flows through `ContainerConfig.Architecture` to Docker's image-pull + container-create `Platform` option.

Full host-model spec: [`specs/CLOUD_RESOURCE_MAPPING.md § Simulator host model`](../specs/CLOUD_RESOURCE_MAPPING.md#simulator-host-model-phase-135).

Stdout/stderr is captured in real time and injected into the cloud-native log sink:

| Service | Log sink | API for retrieval |
|---|---|---|
| ECS | CloudWatch Logs (awslogs) | `GetLogEvents` / `FilterLogEvents` |
| Cloud Run Jobs | Cloud Logging | `entries.list` (REST) / `ListLogEntries` (gRPC) |
| Container Apps | Log Analytics | KQL via `QueryWorkspace` |
| Lambda | CloudWatch Logs | `GetLogEvents` |
| Cloud Functions | Cloud Logging | `entries.list` / `ListLogEntries` |
| Azure Functions | Log Analytics (AppTraces) | KQL via `QueryWorkspace` |

FaaS simulators (Lambda, Cloud Functions, Azure Functions) also execute real processes when `SimCommand` is set, returning the result synchronously.

## ECS ExecuteCommand

The ECS simulator supports `ExecuteCommand` with WebSocket-based session bridging:

1. Spawn a new process with the given command.
2. Register a WebSocket handler at `/ecs-exec/{sessionId}`.
3. Return a session with the WebSocket URL.
4. Bridge stdin/stdout/stderr over the WebSocket connection.

## Request routing per cloud

| Cloud | Protocol | Routing |
|---|---|---|
| AWS (ECS, ECR, CloudWatch, Cloud Map, WAFv2, ACM, KMS, SSM, Secrets, DynamoDB) | AWS-JSON 1.1 | `X-Amz-Target` header dispatch |
| AWS (EC2, IAM, STS) | AWS Query | `Action` form parameter dispatch |
| AWS (Lambda, S3, EFS, CloudFront, Route 53, Amplify) | REST | Path-based mux (CloudFront / Route 53 use XML bodies, others JSON) |
| GCP (all services) | REST + gRPC | Path-based mux (HTTP), proto service (gRPC on port+1 for Cloud Logging) |
| Azure (all services) | ARM REST | Path-based mux with `api-version` validation |

## Known issues

None open. Cross-cloud quirks (Cloud Run lacking `BackingPDEphemeral`, Azure terraform-tests being Docker-only, etc.) are documented in each per-cloud README and in [`PLAN.md`](../PLAN.md). Active bugs land in [`BUGS.md`](../BUGS.md) the moment they surface.

## What's out of scope

- **Cloud-side production deployments.** Simulators are for local dev + CI. For real cloud deployments use the actual cloud APIs through the [Sockerless backends](../backends/).
- **Multi-region / cross-region replication.** Each sim is single-region; multi-region routing belongs to real cloud infra.
- **Billing / pricing / quota surfaces.** Absent except where load-bearing for testing (e.g. `SIM_GCP_CPU_QUOTA_PER_REGION` for Cloud Run quota-rejection tests).
- **Real authentication.** Bearer tokens / SigV4 / OAuth tokens are accepted but not cryptographically verified.
- **DNS resolution at the UDP/53 layer.** Sims store records but don't serve DNS queries. Pair with dnsmasq if you need real lookups.

## Per-cloud guides

| Cloud | CLI | Terraform | Python SDK |
|---|---|---|---|
| AWS | [AWS CLI](aws/docs/cli.md) | [`hashicorp/aws`](aws/docs/terraform.md) | [boto3](aws/docs/python-sdk.md) |
| GCP | [gcloud CLI](gcp/docs/cli.md) | [`hashicorp/google`](gcp/docs/terraform.md) | [`google-cloud-*`](gcp/docs/python-sdk.md) |
| Azure | [az CLI](azure/docs/cli.md) | [`hashicorp/azurerm`](azure/docs/terraform.md) | [`azure-mgmt-*`](azure/docs/python-sdk.md) |

See also: [`backends/*/README.md`](../backends/) for the consumers of each simulator, [`specs/CLOUD_RESOURCE_MAPPING.md`](../specs/CLOUD_RESOURCE_MAPPING.md) for "how does sockerless model X on cloud Y", [`docs/POD_MATERIALIZATION.md`](../docs/POD_MATERIALIZATION.md) for the container-to-cloud-resource walkthrough per backend, [`AGENTS.md`](../AGENTS.md#simulator-architecture--cloud-slice-principle) for the cloud-slice principle.
