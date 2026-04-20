# Running sockerless ECS backend against real AWS

How to stand up the sockerless ECS backend against a real AWS account and expose its Docker API so external runners or clients can use it. This is the prerequisite for `docs/GITHUB_RUNNER_SAAS.md` and `docs/GITLAB_RUNNER_SAAS.md`.

## Prerequisites

- An AWS account with permissions to create VPC, IAM, ECS, ECR, CloudWatch, Cloud Map, and (optionally) EFS resources.
- Terraform 1.6+ and Terragrunt 0.50+.
- Docker on your local machine to build the backend binary + image.
- A domain (or static IP) if you want external runners to reach the sockerless endpoint from outside the VPC.

## Infrastructure — Terraform

The repository ships Terraform at `terraform/modules/ecs/` with a Terragrunt wrapper at `terraform/environments/ecs/live/terragrunt.hcl`. The live environment creates:

- VPC (`10.99.0.0/16`), two public subnets, an internet gateway.
- ECS cluster named `sockerless-live-cluster`.
- IAM role for Fargate task execution (pulls from ECR, writes CloudWatch logs).
- IAM role for the sockerless control plane (reads ECS, writes Cloud Map, manages ENIs).
- ECR repository for overlay images.
- CloudWatch log group `/sockerless/ecs/live` with 7-day retention.
- Cloud Map namespace scaffolding is created per sockerless network at runtime; Terraform doesn't pre-provision namespaces.

Bring it up:

```bash
cd terraform/environments/ecs/live
terragrunt init
terragrunt apply
```

The outputs include the role ARNs, subnet IDs, and cluster name — feed them into the backend via env vars (see below).

## Running the backend

On a reachable host — an EC2 instance, an ECS Fargate service, or even a local machine with ngrok/tailscale — run the `sockerless-backend-ecs` binary with env vars pointing at the live cluster:

```bash
export AWS_REGION=eu-west-1
export SOCKERLESS_ECS_CLUSTER=sockerless-live-cluster
export SOCKERLESS_ECS_SUBNETS=subnet-abc,subnet-def
export SOCKERLESS_ECS_EXECUTION_ROLE_ARN=arn:aws:iam::123456789012:role/sockerless-live-exec
export SOCKERLESS_ECS_TASK_ROLE_ARN=arn:aws:iam::123456789012:role/sockerless-live-task
export SOCKERLESS_ECS_LOG_GROUP=/sockerless/ecs/live
# Leave SOCKERLESS_ENDPOINT_URL unset — live mode talks to real AWS.

sockerless-backend-ecs --addr 0.0.0.0:9100
```

Then run the Docker frontend, which exposes the Docker REST API and delegates to the backend:

```bash
sockerless-docker-frontend \
  --addr 0.0.0.0:2375 \
  --backend http://127.0.0.1:9100
```

Test end-to-end:

```bash
export DOCKER_HOST=tcp://<host>:2375
docker run --rm alpine echo hello-from-fargate
```

This should register a task definition, run a Fargate task in your VPC, stream logs from CloudWatch, and return `hello-from-fargate` to stdout.

## Exposing the Docker endpoint securely

A plain TCP `:2375` socket is fine for same-host or same-VPC use. For external CI runners, front it with TLS. Options:

| Option | When | Trade-off |
|---|---|---|
| Local-only socket + SSH tunnel from runner host | Single-user dev / POC | Per-runner tunnel setup. No exposed endpoint. |
| ALB + ACM cert + client cert (mTLS) | Multi-runner, trusted network | Most robust; requires managing a CA. |
| Cloudflare Tunnel / ngrok + token auth | Quick external reach | Third-party dependency. |
| VPN (Tailscale, WireGuard) to the VPC | Mixed internal + external | Runner joins VPN; no public endpoint. |

sockerless does not ship TLS termination; use a reverse proxy or front service. Do **not** expose `tcp://…:2375` on a public IP without auth — the Docker REST API has no built-in authentication.

## Docker Hub rate limits

Image pulls go through ECR pull-through cache (`backends/lambda/image_resolve.go` and ECS equivalent). First pull of each tag touches Docker Hub via ECR's upstream configuration — supply Docker Hub credentials via `SOCKERLESS_DOCKER_HUB_TOKEN` to avoid the 100-pulls-per-6h anonymous limit. The Terraform module wires this as an ECR secret.

## Verification before using with runners

Run the live-mode Docker CLI smoke test suite:

```bash
cd tests/e2e-live-tests
./start-backend.sh --backend ecs --mode live --backend-addr 0.0.0.0:2375 --pidfile /tmp/skls.pids
# in another terminal:
DOCKER_HOST=tcp://127.0.0.1:2375 docker run --rm alpine uname -a
DOCKER_HOST=tcp://127.0.0.1:2375 docker ps -a
```

`PLAN_ECS_MANUAL_TESTING.md` has the full Round-1 through Round-6 smoke matrix for what's been validated live.

## Troubleshooting

- **`RUN` verb in Dockerfile is a no-op** — expected. ECS Fargate only runs pre-built images. Use the CodeBuild path (`SOCKERLESS_AWS_CODEBUILD_PROJECT`) to actually build images in-cloud.
- **`docker exec` hangs** — ECS ExecuteCommand requires an SSM Session Manager setup. Confirm the task role has `ssmmessages:*` permissions (already in the Terraform module).
- **Cross-container DNS (`postgres:5432` unreachable from another container)** — verify Cloud Map namespace was created for the network. See `docs/ECS_SERVICES_DESIGN.md` for the underlying mechanics.
- **Logs not appearing** — ECS writes to the group named by `awslogs-group` in the task definition. Stream name format: `<container-id-12>/main/<task-id>`.

## Cost guardrails

Fargate billed per task-second plus vCPU/memory. A sockerless-backed CI runner with no job queue will run 1 task per `docker run` — short-lived tasks incur the 1-minute minimum billing. For cost-sensitive setups, batch or pool jobs at the runner level.
