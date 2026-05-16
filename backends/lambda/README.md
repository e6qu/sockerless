# Lambda Backend

Runs Docker containers as AWS Lambda functions using container images, with CloudWatch Logs for log streaming. Frontend speaks Docker REST API v1.44; backend speaks the Lambda / CloudWatch Logs / ECR / IAM APIs.

## Reference adaptors

| Direction | Adaptor | Min version | What it proves |
|---|---|---|---|
| **Frontend (Docker API)** | [Docker Go SDK](https://pkg.go.dev/github.com/docker/docker/client) | v25+ | `docker run` → Lambda invoke round-trip via `tcp://localhost:3375`. |
| | [`docker` CLI](https://docs.docker.com/engine/reference/commandline/cli/) | 29.x | Wire-level [Docker REST API v1.44](https://docs.docker.com/engine/api/v1.44/). |
| **Backend (AWS API)** | [`aws` CLI](https://docs.aws.amazon.com/cli/latest/reference/lambda/) | v2.17+ | `aws lambda invoke`, `aws lambda get-function`, `aws logs tail` — operators inspect function state. |
| | [AWS Go SDK v2](https://github.com/aws/aws-sdk-go-v2/tree/main/service/lambda) | v1.50+ | `lambda.CreateFunction`, `lambda.Invoke` with `LogType=Tail`. The Invoke-diagnostics pattern (`LogType=Tail` + payload dump for crashes) lives in [`memory/feedback_lambda_invoke_diagnostics.md`](../../). |
| | [Terraform `aws` provider](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/lambda_function) | v6.32+ | `aws_lambda_function` provisions the function infra; `simulators/aws/terraform-tests/` covers the path. |

Local development and CI replace the backend-side upstream with [`simulators/aws`](../../simulators/aws/README.md). The agent-as-handler model unique to Lambda is described in [`docs/POD_MATERIALIZATION.md § Lambda`](../../docs/POD_MATERIALIZATION.md).

## Validation

| Test path | What runs | Last green |
|---|---|---|
| `tests/` (Docker SDK against running backend, Lambda profile) | Container lifecycle round-trip through Lambda Invoke. | 2026-05-13 |
| `simulators/aws/sdk-tests/` Lambda package | `CreateFunction` / `Invoke` / `GetFunction` wire shapes validated against sim. | 2026-05-15 |
| `simulators/aws/terraform-tests/` | `aws_lambda_function` apply / destroy round-trip. | 2026-05-15 |
| `make backends/lambda/test` | Leaf-Makefile unit + integration suite. | 2026-05-13 |

## Wiring the adaptor

```bash
cd backends/lambda && make build
./sockerless-backend-lambda --addr :3375 --log-level info &
export DOCKER_HOST=tcp://localhost:3375
```

### Config (config.yaml)

```yaml
environments:
  my-lambda:
    backend: lambda
    addr: ":3375"
    log_level: info
    aws:
      region: us-east-1
      lambda:
        role_arn: arn:aws:iam::123456789012:role/sockerless-lambda
        log_group: /sockerless/lambda
        memory_size: 1024
        timeout: 900
        subnets: [subnet-0a1b2c3d]
        security_groups: [sg-012abc34]
    common:
      callback_url: https://backend.example.com
      poll_interval: 2s
      agent_timeout: 30s
```

Full schema: [`specs/CONFIG.md`](../../specs/CONFIG.md).

### Environment Variables

| Variable | Default | Required | Description |
|---|---|---|---|
| `AWS_REGION` | `us-east-1` | no | AWS region |
| `SOCKERLESS_LAMBDA_ROLE_ARN` | | **yes** | IAM execution role ARN for Lambda functions |
| `SOCKERLESS_LAMBDA_LOG_GROUP` | `/sockerless/lambda` | no | CloudWatch log group |
| `SOCKERLESS_LAMBDA_MEMORY_SIZE` | `1024` | no | Function memory in MB |
| `SOCKERLESS_LAMBDA_TIMEOUT` | `900` | no | Function timeout in seconds (max 900) |
| `SOCKERLESS_LAMBDA_SUBNETS` | | no | Comma-separated subnet IDs for VPC mode |
| `SOCKERLESS_LAMBDA_SECURITY_GROUPS` | | no | Comma-separated security group IDs |
| `SOCKERLESS_CALLBACK_URL` | | no | Backend URL for reverse agent callbacks |
| `SOCKERLESS_ENDPOINT_URL` | | no | Custom AWS endpoint (for [`simulators/aws`](../../simulators/aws/README.md)) |
| `SOCKERLESS_POLL_INTERVAL` | `2s` | no | Cloud API poll interval |
| `SOCKERLESS_AGENT_TIMEOUT` | `30s` | no | Agent health-check timeout |

CLI flags: `-addr` (default `:3375`), `-tls-cert`, `-tls-key`, `-log-level` (default `info`).

## Sample

```bash
$ DOCKER_HOST=tcp://localhost:3375 docker run --rm alpine:3.20 echo "hello from lambda"
hello from lambda

$ aws lambda invoke --function-name sockerless-fn-abc /tmp/out --log-type Tail
{"StatusCode": 200, "LogResult": "..."}
$ cat /tmp/out
hello from lambda
```

## Known issues

None open. **Lambda has no invoke-cancel API**: `UpdateFunctionConfiguration(Timeout=…)` applies only to future invocations; in-flight invocations continue to completion. The cooperative-termination path (agent-as-handler) is the only stop signal — see [`docs/POD_MATERIALIZATION.md § Lambda`](../../docs/POD_MATERIALIZATION.md).

## What's out of scope

- Native runtime modes (Node, Python, Java). This backend uses **container mode only** — see [`memory/feedback_faas_container_mode.md`](../../).
- Provisioned concurrency (cold-start optimisation belongs to the operator-Terraform layer).
- Lambda@Edge / SnapStart.

## Cloud Notes

- The execution role needs `lambda:*`, `logs:*`, and ECR pull permissions at minimum.
- Uses reverse agent exclusively — Lambda cannot accept inbound connections.
- VPC subnets/security groups are only needed if functions must reach private resources.
- Lambda timeout max is 900 seconds (15 minutes). Container images must be in ECR.

See also: [`backends/aws-common`](../aws-common/), [`simulators/aws/API_SPEC.md § Lambda`](../../simulators/aws/API_SPEC.md), [`specs/CLOUD_RESOURCE_MAPPING.md`](../../specs/CLOUD_RESOURCE_MAPPING.md).
