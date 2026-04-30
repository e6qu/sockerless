# Infrastructure setup

Per-cloud terraform / terragrunt environments that bring up the live infra each runbook needs. All environments are designed for short, time-boxed sweeps — provision, run the runbook, destroy.

## AWS (eu-west-1)

Shared VPC; Lambda reuses the ECS subnets + security group.

### Image source policy

Sockerless on AWS deliberately avoids Docker Hub credentials. Three routes are supported, all credential-free:

- **AWS Public Gallery** (`public.ecr.aws/...`) — pullable directly by Fargate / Lambda. Sockerless's `resolveImageURI` rewrites Docker Hub library refs (`alpine`, `node:20`, `nginx:alpine`) to `public.ecr.aws/docker/library/<name>:<tag>` automatically, so a workflow's `image: alpine:latest` resolves to AWS's official Docker Hub mirror without operator setup.
- **ECR pull-through cache** for other public registries (`ghcr.io/...`, `quay.io/...`, `registry.k8s.io/...`, `mcr.microsoft.com/...`). The backend creates the cache rule on first pull; no credentials.
- **Operator-owned ECR** for everything else — push your private images to the ECR repository this terraform creates (`docker push <ecr-uri>`), then reference the ECR URI directly.

Docker Hub user/org images (e.g. `myorg/myapp:v1`) are rejected with a clear error pointing the operator at `docker push <ecr-uri>` — the project's no-credentials-on-disk discipline avoids Docker Hub PATs by design.

### Provision

```bash
# ECS infrastructure: VPC, subnets, SGs, cluster, EFS, ECR, IAM, Cloud Map.
cd terraform/environments/ecs/live
source aws.sh && terragrunt apply -auto-approve
terragrunt output -json | python3 -c "
import json, sys
d = json.load(sys.stdin)
for k,v in {
  'AWS_REGION': d['region']['value'],
  'SOCKERLESS_ECS_CLUSTER': d['ecs_cluster_name']['value'],
  'SOCKERLESS_ECS_SUBNETS': ','.join(d['private_subnet_ids']['value']),
  'SOCKERLESS_ECS_SECURITY_GROUPS': d['task_security_group_id']['value'],
  'SOCKERLESS_ECS_TASK_ROLE_ARN': d['task_role_arn']['value'],
  'SOCKERLESS_ECS_EXECUTION_ROLE_ARN': d['execution_role_arn']['value'],
  'SOCKERLESS_ECS_LOG_GROUP': d['log_group_name']['value'],
  'SOCKERLESS_AGENT_EFS_ID': d['efs_filesystem_id']['value'],
  'SOCKERLESS_ECS_PUBLIC_IP': 'true',
}.items(): print(f'export {k}={v}')
" > /tmp/ecs-env.sh

# Lambda infrastructure: IAM role, log group, ECR repo.
cd terraform/environments/lambda/live
source aws.sh && terragrunt apply -auto-approve
LAMBDA_ROLE=$(terragrunt output -json | python3 -c "import json,sys;print(json.load(sys.stdin)['execution_role_arn']['value'])")
```

### AWS access key state

The root-account access key is currently **deactivated**. Reactivate via AWS Console (`IAM → Security credentials → Access keys`) before running any AWS sweep — root-account keys can't be touched via the IAM API.

### AWS teardown

```bash
source aws.sh

# Stop running tasks first (takes ~60s).
for task in $(aws ecs list-tasks --cluster sockerless-live --region eu-west-1 --query 'taskArns[]' --output text); do
  aws ecs stop-task --cluster sockerless-live --task "$task" --region eu-west-1
done
sleep 60

# Destroy.
cd terraform/environments/ecs/live    && terragrunt destroy -auto-approve
cd terraform/environments/lambda/live && terragrunt destroy -auto-approve

# Deregister orphan task definitions.
for td in $(aws ecs list-task-definitions --region eu-west-1 --status ACTIVE \
  --query 'taskDefinitionArns[?contains(@,`sockerless`)]' --output text); do
  aws ecs deregister-task-definition --task-definition "$td" --region eu-west-1
done

# Optional: remove tf state bucket.
aws s3 rb s3://sockerless-tf-state --region eu-west-1 --force
```

The per-cloud `null_resource sockerless_runtime_sweep` in each terraform module makes `terragrunt destroy` self-sufficient — no manual cleanup of runtime-created sockerless resources is needed beyond what's shown above.

## GCP

Live env queued — needs project + VPC connector. Terraform module to be added under `terraform/environments/cloudrun/live/` and `cloudrun-functions/live/`.

## Azure

Live env queued — needs subscription + managed environment with VNet integration. Terraform module to be added under `terraform/environments/aca/live/` and `azure-functions/live/`.

## Backend startup

```bash
# Build backends.
cd backends/ecs    && go build -o sockerless-backend-ecs    ./cmd/sockerless-backend-ecs
cd backends/lambda && go build -o sockerless-backend-lambda ./cmd/sockerless-backend-lambda

# ECS backend on :3375.
source aws.sh && source /tmp/ecs-env.sh
./sockerless-backend-ecs -addr :3375 &

# Lambda backend on :9200 (shares VPC from ECS).
export SOCKERLESS_LAMBDA_ROLE_ARN=$LAMBDA_ROLE
export SOCKERLESS_LAMBDA_SUBNETS=$SOCKERLESS_ECS_SUBNETS
export SOCKERLESS_LAMBDA_SECURITY_GROUPS=$SOCKERLESS_ECS_SECURITY_GROUPS
# Enable docker commit on Lambda (opt-in):
export SOCKERLESS_ENABLE_COMMIT=1
# Enable docker exec/attach/pause/cp via reverse-agent:
export SOCKERLESS_CALLBACK_URL="wss://<public-endpoint-for-lambda>/v1/lambda/reverse"
./sockerless-backend-lambda -addr :9200 &

# Docker CLI against either backend.
export DOCKER_HOST=tcp://localhost:3375  # ECS
# or: export DOCKER_HOST=tcp://localhost:9200  # Lambda

# Podman CLI.
podman system connection add sockerless-ecs    tcp://localhost:3375
podman system connection add sockerless-lambda tcp://localhost:9200
```

## Costs

- **NAT Gateway** ~$0.045/hr while the ECS VPC is up. Tear down promptly.
- **EFS** charged per GB-month; round-trip a sweep is sub-$0.01.
- **CloudWatch logs** charged per GB ingested; runner sweeps stay well under the free tier.
- **Lambda** invocations + GB-seconds; full Track D sweep is sub-$0.01.

Total per-session cost for a complete AWS sweep is typically <$1 if you tear down inside an hour.
