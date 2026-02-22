# backend-lambda

AWS Lambda backend. Maps Docker container operations to Lambda functions using container images.

## Resource mapping

| Docker concept | Lambda resource |
|---------------|----------------|
| Container create | `CreateFunction` (container image) |
| Container start | `Invoke` (async) |
| Container stop | No-op (runs to completion) |
| Container kill | Disconnects reverse agent |
| Container remove | `DeleteFunction` |
| Container logs | CloudWatch Logs `GetLogEvents` |

## Agent mode

Uses **reverse agent** exclusively. Lambda functions cannot accept inbound connections, so the agent inside the function dials back to the backend via `SOCKERLESS_CALLBACK_URL`.

Helper and cache containers (those not running `tail -f /dev/null`) auto-stop after 500ms.

## Building

```sh
cd backends/lambda
go build -o sockerless-backend-lambda ./cmd/sockerless-backend-lambda
```

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `AWS_REGION` | `us-east-1` | AWS region |
| `SOCKERLESS_LAMBDA_ROLE_ARN` | _(required)_ | IAM execution role for functions |
| `SOCKERLESS_LAMBDA_LOG_GROUP` | `/sockerless/lambda` | CloudWatch log group |
| `SOCKERLESS_LAMBDA_MEMORY_SIZE` | `1024` | Function memory (MB) |
| `SOCKERLESS_LAMBDA_TIMEOUT` | `900` | Function timeout (seconds) |
| `SOCKERLESS_LAMBDA_SUBNETS` | | Comma-separated subnet IDs (VPC mode) |
| `SOCKERLESS_LAMBDA_SECURITY_GROUPS` | | Comma-separated security group IDs |
| `SOCKERLESS_CALLBACK_URL` | | Backend URL for reverse agent connections |
| `SOCKERLESS_ENDPOINT_URL` | | Custom AWS endpoint (simulator mode) |

## Project structure

```
lambda/
├── cmd/sockerless-backend-lambda/
│   └── main.go          CLI entrypoint
├── server.go            Server type, route overrides
├── config.go            Config struct, env parsing, validation
├── aws.go               AWS SDK client initialization
├── containers.go        Create, start, stop, kill, remove handlers
├── logs.go              CloudWatch Logs streaming
├── images.go            Image pull/load handlers
├── extended.go          Container prune
├── store.go             LambdaState type
└── errors.go            AWS error mapping
```

## Example deployment

See [examples/terraform/](examples/terraform/) for a complete Terraform example that provisions the AWS infrastructure (IAM roles, CloudWatch, ECR) and walks through running Docker commands against Lambda.

## Docker API mapping

For a detailed breakdown of how each Docker REST API endpoint and CLI command maps to AWS Lambda operations — including what's supported, what's not, and how it compares to vanilla Docker — see [docs/docker_api_mapping.md](docs/docker_api_mapping.md).

## Testing

```sh
make sim-test-aws    # simulator integration tests
make docker-test     # Docker-based full test
```
