# Infrastructure setup

Per-cloud terraform / terragrunt environments that bring up the live infra each runbook needs. All environments are designed for short, time-boxed sweeps — provision, run the runbook, destroy.

## AWS (eu-west-1)

Shared VPC; Lambda reuses the ECS subnets + security group.

### Prerequisite: Docker Hub credentials in Secrets Manager

The ECS backend pulls Docker Hub images (`alpine:latest`, `node:20`, etc.) through an ECR pull-through cache. AWS requires a Secrets Manager secret containing valid Docker Hub credentials for the cache rule to mint pull tokens — this is a real cloud requirement, not a sockerless choice. Without it, the very first `docker run alpine:latest` fails with `UnsupportedUpstreamRegistryException` (per BUG-708 the backend surfaces this clearly rather than falling back).

One-time setup per AWS account:

```bash
# 1. Mint a Docker Hub access token at
#    https://hub.docker.com/settings/security → "New Access Token"
#    Scope: Public Repo Read-only. Copy the token.

# 2. Create the secret. The name MUST start with `ecr-pullthroughcache/`
#    — the AWSServiceRoleForECRPullThroughCache role only has read
#    permission on secrets matching that prefix (per AWS docs), and
#    without that prefix every pull-through call fails 400 even with
#    a valid PAT.
read -s DH_TOKEN  # paste; hit enter
read -p "Docker Hub username: " DH_USER

aws secretsmanager create-secret \
  --name ecr-pullthroughcache/sockerless-dockerhub \
  --description "Docker Hub PAT for ECR pull-through cache (sockerless)" \
  --region eu-west-1 \
  --secret-string "{\"username\":\"$DH_USER\",\"accessToken\":\"$DH_TOKEN\"}"
unset DH_TOKEN

# 3. Capture the ARN — backend reads it via SOCKERLESS_ECR_DOCKERHUB_CREDENTIAL_ARN.
export SOCKERLESS_ECR_DOCKERHUB_CREDENTIAL_ARN=$(aws secretsmanager describe-secret \
  --secret-id ecr-pullthroughcache/sockerless-dockerhub --region eu-west-1 --query ARN --output text)
```

Once this secret exists you can persist the ARN export in `aws.sh` so future sessions pick it up automatically. The secret stays valid until you rotate the Docker Hub PAT.

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

# Also export the Docker Hub creds ARN from the prereq above.
echo "export SOCKERLESS_ECR_DOCKERHUB_CREDENTIAL_ARN=$SOCKERLESS_ECR_DOCKERHUB_CREDENTIAL_ARN" >> /tmp/ecs-env.sh

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
