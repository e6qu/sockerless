# ECS Backend

Runs Docker containers as AWS ECS Fargate tasks, with CloudWatch Logs for log streaming. Frontend speaks Docker REST API v1.44; backend speaks the ECS / ECR / CloudWatch Logs / EC2 / IAM / Cloud Map APIs.

## Reference adaptors

This backend is a translator. The **frontend** adaptors are the Docker clients that drive it; the **backend** adaptors are the AWS tools that verify what it created (or that drove the infra it sits on top of).

| Direction | Adaptor | Min version | What it proves |
|---|---|---|---|
| **Frontend (Docker API)** | [Docker Go SDK](https://pkg.go.dev/github.com/docker/docker/client) | v25+ | Anything the Docker SDK does against `unix:///var/run/docker.sock` must work against this backend over `tcp://localhost:3375`. Covered by `tests/`. |
| | [`docker` CLI](https://docs.docker.com/engine/reference/commandline/cli/) | 29.x | Wire-level [Docker REST API v1.44](https://docs.docker.com/engine/api/v1.44/). |
| | `podman` CLI | 5.x | Docker-compat shim (`podman --url tcp://localhost:3375 …`). |
| **Backend (AWS API)** | [`aws` CLI](https://docs.aws.amazon.com/cli/latest/reference/ecs/) | v2.17+ | `aws ecs describe-tasks`, `aws logs filter-log-events`, etc. — operators verify task state the same way they would against real ECS. |
| | [AWS Go SDK v2](https://github.com/aws/aws-sdk-go-v2/tree/main/service/ecs) | v1.50+ | `ecs.RunTask`, `cloudwatchlogs.GetLogEvents` — the calls this backend issues. Same calls validated by `simulators/aws/sdk-tests/`. |
| | [Terraform `aws` provider](https://registry.terraform.io/providers/hashicorp/aws/latest/docs) | v6.32+ | The supporting infra (cluster, IAM roles, subnets, log group) is provisioned via real Terraform; covered by `simulators/aws/terraform-tests/`. |

For local development and CI the **backend adaptor's upstream is replaced** by [`simulators/aws`](../../simulators/aws/README.md) — same wire shapes, no AWS account needed.

## Validation

| Test path | What runs | Last green |
|---|---|---|
| `tests/` (Docker SDK against running backend) | Real Docker Go SDK exercising 59 functions — containers / images / volumes / networks / exec / logs / attach round-trip. | 2026-05-13 |
| `tests/github_runner_e2e_test.go` | Official `actions/runner` driving Docker REST against this backend, with jobs executing inside ECS tasks. | 2026-05-13 |
| `simulators/aws/sdk-tests/` ECS package | The AWS-side calls this backend makes (`RunTask`, `DescribeTasks`, `StopTask`, etc.) validated against the sim. | 2026-05-15 |
| `simulators/aws/terraform-tests/TestStackProductionShape` | Cross-resource Terraform plan including `aws_ecs_cluster` — proves the operator-provisioning path. | 2026-05-15 |
| `make backends/ecs/test` | The leaf-Makefile unit + integration suite per [`docs/MAKEFILE_STANDARD.md`](../../docs/MAKEFILE_STANDARD.md). | 2026-05-13 |

## Wiring the adaptor

```bash
# 1. Build + start the backend.
cd backends/ecs && make build
./sockerless-backend-ecs --addr :3375 --log-level info &

# 2. Point any Docker client at it.
export DOCKER_HOST=tcp://localhost:3375
```

### Config (config.yaml)

```yaml
environments:
  my-ecs:
    backend: ecs
    addr: ":3375"
    log_level: info
    simulator: aws-sim          # optional, for local dev — points at simulators/aws
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

Full YAML schema: [`specs/CONFIG.md`](../../specs/CONFIG.md).

### Environment Variables

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
| `SOCKERLESS_ENDPOINT_URL` | | no | Custom AWS endpoint (for [`simulators/aws`](../../simulators/aws/README.md)) |
| `SOCKERLESS_POLL_INTERVAL` | `2s` | no | Cloud API poll interval |
| `SOCKERLESS_AGENT_TIMEOUT` | `30s` | no | Agent health-check timeout |

CLI flags: `-addr` (default `:3375`), `-tls-cert`, `-tls-key`, `-log-level` (default `info`).

## Sample

End-to-end via the `docker` CLI driving a real ECS task (or sim-backed during local dev):

```bash
$ DOCKER_HOST=tcp://localhost:3375 docker run --rm alpine:3.20 echo "hello from ecs"
hello from ecs

# Verify via the backend adaptor:
$ aws ecs list-tasks --cluster sockerless --desired-status STOPPED
{
    "taskArns": ["arn:aws:ecs:us-east-1:000000000000:task/sockerless/abc..."]
}

$ aws logs filter-log-events --log-group-name /sockerless --limit 5
{
    "events": [{"message": "hello from ecs", ...}]
}
```

[`docs/POD_MATERIALIZATION.md § ECS`](../../docs/POD_MATERIALIZATION.md) walks through the full container → task translation.

## Known issues

None open. Backend-API quirks are catalogued in [`docs/RUNNERS.md § Runner hurdles`](../../docs/RUNNERS.md); the cross-resource mapping (Docker container → ECS task definition + task) is documented in [`specs/CLOUD_RESOURCE_MAPPING.md`](../../specs/CLOUD_RESOURCE_MAPPING.md).

## What's out of scope

- ECS EC2 launch type (Fargate only).
- ECS Service / Service-Connect orchestration (this backend creates Tasks; long-running services belong to a different layer).
- Real Docker image builds (use a separate builder backend; this one expects images already in ECR).
- IAM provisioning (the cluster + execution + task roles must pre-exist; sockerless does not create them).

## Cloud Notes

- Requires an ECS cluster, at least one VPC subnet, and an execution role with ECR pull + CloudWatch Logs permissions.
- The task role needs permissions for any AWS services your containers access.
- Set `assign_public_ip: true` if tasks run in public subnets without a NAT gateway.
- Supports forward agent (polls ENI for IP) and reverse agent (`callback_url`).

See also: [`backends/aws-common`](../aws-common/) (shared `AuthProvider`), [`simulators/aws/API_SPEC.md`](../../simulators/aws/API_SPEC.md) for the AWS-side wire shapes.
