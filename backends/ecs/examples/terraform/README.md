# ECS Backend — Terraform Example

This example provisions the AWS infrastructure needed to run Sockerless with the ECS Fargate backend. Once applied, you can use standard `docker` CLI commands and they will execute as ECS Fargate tasks.

## What Gets Created

- **VPC** with public and private subnets across 2 availability zones
- **NAT Gateway** for outbound internet from private subnets
- **ECS Cluster** (Fargate) with Container Insights enabled
- **EFS Filesystem** with mount targets in each private subnet
- **CloudWatch Log Group** for container logs
- **ECR Repository** for container images
- **IAM Roles** — execution role (image pull + logs) and task role (EFS + CloudWatch)
- **Security Groups** — task SG (port 9111 for agent, intra-group traffic) and EFS SG (NFS 2049)
- **Cloud Map Namespace** for service discovery

## Prerequisites

- [Terraform](https://developer.hashicorp.com/terraform/install) >= 1.5
- [AWS CLI](https://aws.amazon.com/cli/) configured with credentials (`aws configure`)
- Go 1.24+ (to build the backend binary)

## Step 1: Apply the Terraform

```bash
cd backends/ecs/examples/terraform

terraform init
terraform plan
terraform apply
```

Review the plan and type `yes` to confirm. This takes approximately 3-5 minutes (NAT Gateway and EFS are the slowest resources).

To customize the deployment:

```bash
# Different region
terraform apply -var="region=eu-west-1" -var='availability_zones=["eu-west-1a","eu-west-1b"]'

# Custom project name
terraform apply -var="project_name=myproject" -var="environment=staging"
```

## Step 2: Export the Backend Configuration

After `terraform apply` completes, export the environment variables the backend needs:

```bash
# Quick method — copy-paste the output
terraform output -raw backend_env

# Or export them one by one
export AWS_REGION=$(terraform output -raw region 2>/dev/null || echo "us-east-1")
export SOCKERLESS_ECS_CLUSTER=$(terraform output -raw ecs_cluster_name)
export SOCKERLESS_ECS_SUBNETS=$(terraform output -raw private_subnet_ids)
export SOCKERLESS_ECS_SECURITY_GROUPS=$(terraform output -raw task_security_group_id)
export SOCKERLESS_ECS_EXECUTION_ROLE_ARN=$(terraform output -raw execution_role_arn)
export SOCKERLESS_ECS_TASK_ROLE_ARN=$(terraform output -raw task_role_arn)
export SOCKERLESS_ECS_LOG_GROUP=$(terraform output -raw log_group_name)
```

## Step 3: Build and Run the Backend

```bash
# Build the backend binary
cd backends/ecs
go build -o sockerless-backend-ecs ./cmd/sockerless-backend-ecs

# Run the backend (listens on port 9100 by default)
./sockerless-backend-ecs -addr :9100
```

The backend is now running locally and will create ECS Fargate tasks in your AWS account when Docker commands are issued.

## Step 4: Configure Docker to Use Sockerless

Point the Docker CLI at the Sockerless frontend:

```bash
# Build and run the frontend (translates Docker API → Sockerless internal API)
cd frontends/docker
go build -o sockerless-frontend-docker .
./sockerless-frontend-docker -backend http://localhost:9100 -addr unix:///tmp/sockerless.sock

# Use docker with the custom socket
export DOCKER_HOST=unix:///tmp/sockerless.sock
```

Now every `docker` command goes through Sockerless to ECS Fargate.

## Step 5: Use Docker Commands

### Pull an image

```bash
docker pull alpine:latest
```

This creates a synthetic image reference in Sockerless. The actual image must be accessible from ECS (e.g., public Docker Hub images or images in the ECR repository).

### Run a container

```bash
# Run a simple command
docker run --rm alpine:latest echo "Hello from ECS Fargate!"

# Run interactively (requires agent)
docker run -it --rm alpine:latest sh
```

Behind the scenes:
1. `docker create` → Sockerless calls `RegisterTaskDefinition`
2. `docker start` → Sockerless calls `RunTask` (Fargate)
3. The agent inside the task connects back, enabling exec/attach
4. `docker rm` → Sockerless calls `StopTask` + cleanup

### Create and manage containers

```bash
# Create a container
docker create --name mycontainer alpine:latest tail -f /dev/null
# → RegisterTaskDefinition

# Start it
docker start mycontainer
# → RunTask (Fargate), waits for agent health check

# Execute commands inside
docker exec mycontainer ls /
docker exec mycontainer cat /etc/os-release
docker exec -it mycontainer sh

# View logs
docker logs mycontainer
docker logs -f mycontainer   # follow mode (polls CloudWatch every 1s)

# Inspect container state
docker inspect mycontainer

# Stop and remove
docker stop mycontainer
docker rm mycontainer
```

### Copy files to/from containers

```bash
# Copy a file into the container (via agent)
echo "hello" > /tmp/test.txt
docker cp /tmp/test.txt mycontainer:/tmp/test.txt

# Copy a file out
docker cp mycontainer:/etc/hostname /tmp/hostname.txt
```

### List and prune

```bash
docker ps           # running containers
docker ps -a        # all containers (including stopped)
docker container prune  # remove stopped containers
```

### Networks and volumes

```bash
# Create a network (in-memory only — ECS uses VPC networking)
docker network create mynet

# Create a volume (in-memory — EFS integration placeholder)
docker volume create myvol
```

Note: Networks and volumes are tracked in memory. ECS tasks use VPC networking (subnets + security groups configured via Terraform). Real persistent volumes would require EFS integration.

## Step 6: Destroy the Infrastructure

When done, tear everything down:

```bash
cd backends/ecs/examples/terraform
terraform destroy
```

Type `yes` to confirm. This takes approximately 3-5 minutes.

**Important:** Make sure all ECS tasks are stopped before destroying. Sockerless containers that are still running may leave orphaned tasks. Check:

```bash
aws ecs list-tasks --cluster sockerless-example
# If any tasks are listed, stop them:
aws ecs stop-task --cluster sockerless-example --task <task-arn>
```

## Architecture Diagram

```
┌──────────────┐     ┌──────────────────┐     ┌────────────────────────┐
│  docker CLI  │────▶│ Sockerless       │────▶│ AWS ECS Fargate        │
│              │     │ Frontend + Backend│     │                        │
│ pull, create,│     │ (localhost:9100)  │     │ RegisterTaskDefinition │
│ start, exec, │     │                  │     │ RunTask                │
│ logs, stop   │     │                  │     │ StopTask               │
└──────────────┘     └──────────────────┘     │ GetLogEvents           │
                                               └────────────────────────┘
```

## Agent Modes

### Forward Agent (default)

The agent runs inside each ECS task. After the task reaches RUNNING state, Sockerless connects to the agent via the task's ENI IP on port 9111. This requires the task security group to allow inbound on port 9111.

### Reverse Agent

Set `SOCKERLESS_CALLBACK_URL` to enable. The agent inside the task connects back to Sockerless. This is useful when the backend is not in the same VPC or when tasks run in private subnets without public IPs.

```bash
export SOCKERLESS_CALLBACK_URL=http://<backend-host>:9100
```

## Estimated Costs

- **NAT Gateway**: ~$0.045/hr (~$32/month)
- **ECS Fargate**: Per-task pricing (256 mCPU + 512 MB per container)
- **CloudWatch Logs**: Per GB ingested
- **EFS**: Per GB stored
- **ECR**: Per GB stored

The NAT Gateway is the largest fixed cost. For development, consider destroying the infrastructure when not in use.

## Troubleshooting

**Tasks fail to start:** Check IAM roles have correct permissions. The execution role needs ECR pull access and CloudWatch Logs write access.

**Agent health check times out:** Ensure the task security group allows inbound on port 9111. Check the task is in a subnet with internet access (via NAT Gateway).

**Logs are empty:** CloudWatch Logs may take a few seconds to appear. The log stream format is `{containerID[:12]}/main/{taskID}`.

**Container image not found:** ECS pulls images at runtime. Ensure the image is accessible from the Fargate task (public images or ECR).
