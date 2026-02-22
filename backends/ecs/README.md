# backend-ecs

AWS ECS Fargate backend. Maps Docker container operations to ECS task definitions and Fargate tasks.

## Resource mapping

| Docker concept | ECS resource |
|---------------|-------------|
| Container create | `RegisterTaskDefinition` |
| Container start | `RunTask` (Fargate) |
| Container stop/kill | `StopTask` |
| Container remove | `StopTask` + `DeregisterTaskDefinition` |
| Container logs | CloudWatch Logs `GetLogEvents` |

## Agent mode

Uses **forward agent** by default: after starting a task, the backend polls the ENI for a public/private IP, then dials into the agent running inside the task.

Also supports **reverse agent** via `SOCKERLESS_CALLBACK_URL`: the agent inside the task dials back to the backend.

## Building

```sh
cd backends/ecs
go build -o sockerless-backend-ecs ./cmd/sockerless-backend-ecs
```

## Configuration

All configuration is via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `AWS_REGION` | `us-east-1` | AWS region |
| `SOCKERLESS_ECS_CLUSTER` | `sockerless` | ECS cluster name |
| `SOCKERLESS_ECS_SUBNETS` | _(required)_ | Comma-separated subnet IDs |
| `SOCKERLESS_ECS_SECURITY_GROUPS` | | Comma-separated security group IDs |
| `SOCKERLESS_ECS_TASK_ROLE_ARN` | | IAM role for the task |
| `SOCKERLESS_ECS_EXECUTION_ROLE_ARN` | _(required)_ | IAM role for task execution (image pull, logs) |
| `SOCKERLESS_ECS_LOG_GROUP` | `/sockerless` | CloudWatch log group |
| `SOCKERLESS_ECS_PUBLIC_IP` | `false` | Assign public IP to tasks |
| `SOCKERLESS_AGENT_IMAGE` | `sockerless/agent:latest` | Sidecar agent image |
| `SOCKERLESS_AGENT_EFS_ID` | | EFS filesystem for agent binary |
| `SOCKERLESS_AGENT_TOKEN` | | Default agent authentication token |
| `SOCKERLESS_CALLBACK_URL` | | Backend URL for reverse agent mode |
| `SOCKERLESS_ENDPOINT_URL` | | Custom AWS endpoint (simulator mode) |

## Project structure

```
ecs/
├── cmd/sockerless-backend-ecs/
│   └── main.go          CLI entrypoint
├── server.go            Server type, route overrides
├── config.go            Config struct, env parsing, validation
├── aws.go               AWS SDK client initialization
├── containers.go        Create, start, stop, kill, remove handlers
├── taskdef.go           ECS task definition builder
├── eni.go               ENI IP extraction from task attachments
├── logs.go              CloudWatch Logs streaming
├── images.go            Image pull/load handlers
├── extended.go          Pause, unpause, volume prune
├── store.go             ECSState, NetworkState, VolumeState types
├── registry.go          Container image registry support
└── errors.go            AWS error mapping
```

## Example deployment

See [examples/terraform/](examples/terraform/) for a complete Terraform example that provisions all the AWS infrastructure (VPC, ECS cluster, IAM roles, CloudWatch, ECR) and walks through running Docker commands against ECS Fargate.

## Docker API mapping

For a detailed breakdown of how each Docker REST API endpoint and CLI command maps to AWS ECS operations — including what's supported, what's not, and how it compares to vanilla Docker — see [docs/docker_api_mapping.md](docs/docker_api_mapping.md).

## Testing

Tested via the AWS simulator:

```sh
make sim-test-aws    # simulator integration tests
make docker-test     # Docker-based full test
```
