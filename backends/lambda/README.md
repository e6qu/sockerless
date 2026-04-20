# Lambda Backend

Runs Docker containers as AWS Lambda functions using container images, with CloudWatch Logs for log streaming.

## Config (config.yaml)

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

## Environment Variables

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
| `SOCKERLESS_ENDPOINT_URL` | | no | Custom AWS endpoint (for simulators) |
| `SOCKERLESS_POLL_INTERVAL` | `2s` | no | Cloud API poll interval |
| `SOCKERLESS_AGENT_TIMEOUT` | `30s` | no | Agent health-check timeout |

## Quick Start

```sh
go build -o sockerless-backend-lambda ./backends/lambda/cmd/sockerless-backend-lambda
./sockerless-backend-lambda -addr :3375 -log-level info
```

Flags: `-addr` (default `:3375`), `-tls-cert`, `-tls-key`, `-log-level` (default `info`).

## Cloud Notes

- The execution role needs `lambda:*`, `logs:*`, and ECR pull permissions at minimum.
- Uses reverse agent exclusively -- Lambda cannot accept inbound connections.
- VPC subnets/security groups are only needed if functions must reach private resources.
- Lambda timeout max is 900 seconds (15 minutes). Container images must be in ECR.
- See `specs/CONFIG.md` for the full unified config specification.
