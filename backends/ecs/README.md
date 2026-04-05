# ECS Backend

Runs Docker containers as AWS ECS Fargate tasks, with CloudWatch Logs for log streaming.

## Config (config.yaml)

```yaml
environments:
  my-ecs:
    backend: ecs
    addr: ":9100"
    log_level: info
    simulator: aws-sim          # optional, for local dev
    aws:
      region: us-east-1
      ecs:
        cluster: sockerless
        subnets: [subnet-0a1b2c3d, subnet-4e5f6a7b]
        security_groups: [sg-012abc34]
        task_role_arn: arn:aws:iam::123456789012:role/sockerless-task
        execution_role_arn: arn:aws:iam::123456789012:role/sockerless-exec
        log_group: /sockerless
        assign_public_ip: false
        agent_efs_id: fs-01234567
    common:
      agent_image: sockerless/agent:latest
      agent_token: my-secret-token
      callback_url: https://backend.example.com
      poll_interval: 2s
      agent_timeout: 30s
```

## Environment Variables

| Variable | Default | Required | Description |
|---|---|---|---|
| `AWS_REGION` | `us-east-1` | no | AWS region |
| `SOCKERLESS_ECS_CLUSTER` | `sockerless` | no | ECS cluster name |
| `SOCKERLESS_ECS_SUBNETS` | | **yes** | Comma-separated subnet IDs |
| `SOCKERLESS_ECS_SECURITY_GROUPS` | | no | Comma-separated security group IDs |
| `SOCKERLESS_ECS_TASK_ROLE_ARN` | | no | IAM role ARN for task containers |
| `SOCKERLESS_ECS_EXECUTION_ROLE_ARN` | | **yes** | IAM role ARN for ECS agent (image pull, logs) |
| `SOCKERLESS_ECS_LOG_GROUP` | `/sockerless` | no | CloudWatch log group |
| `SOCKERLESS_ECS_PUBLIC_IP` | `false` | no | Assign public IP (`true`/`false`) |
| `SOCKERLESS_AGENT_IMAGE` | `sockerless/agent:latest` | no | Sidecar agent container image |
| `SOCKERLESS_AGENT_EFS_ID` | | no | EFS filesystem ID for agent binary |
| `SOCKERLESS_AGENT_TOKEN` | | no | Agent authentication token |
| `SOCKERLESS_CALLBACK_URL` | | no | Backend URL for reverse agent mode |
| `SOCKERLESS_ENDPOINT_URL` | | no | Custom AWS endpoint (for simulators) |
| `SOCKERLESS_POLL_INTERVAL` | `2s` | no | Cloud API poll interval |
| `SOCKERLESS_AGENT_TIMEOUT` | `30s` | no | Agent health-check timeout |

## Quick Start

```sh
go build -o sockerless-backend-ecs ./backends/ecs/cmd/sockerless-backend-ecs
./sockerless-backend-ecs -addr :9100 -log-level info
```

Flags: `-addr` (default `:9100`), `-tls-cert`, `-tls-key`, `-log-level` (default `info`).

## Cloud Notes

- Requires an ECS cluster, at least one VPC subnet, and an execution role with ECR pull + CloudWatch Logs permissions.
- The task role needs permissions for any AWS services your containers access.
- Set `assign_public_ip: true` if tasks run in public subnets without a NAT gateway.
- Supports forward agent (polls ENI for IP) and reverse agent (`callback_url`).
- See `specs/CONFIG.md` for the full unified config specification.
