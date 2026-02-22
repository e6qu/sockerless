# Lambda Backend — Terraform Example

This example provisions the AWS infrastructure needed to run Sockerless with the Lambda backend. Once applied, you can use standard `docker` CLI commands and they will execute as Lambda function invocations.

## What Gets Created

- **IAM Execution Role** for Lambda functions (CloudWatch Logs + ECR pull permissions)
- **CloudWatch Log Group** for function logs
- **ECR Repository** for container images

## Prerequisites

- [Terraform](https://developer.hashicorp.com/terraform/install) >= 1.5
- [AWS CLI](https://aws.amazon.com/cli/) configured with credentials (`aws configure`)
- Go 1.24+ (to build the backend binary)
- Container images pushed to the ECR repository (Lambda requires ECR-hosted images)

## Step 1: Apply the Terraform

```bash
cd backends/lambda/examples/terraform

terraform init
terraform plan
terraform apply
```

This takes approximately 30 seconds (only IAM roles, log groups, and ECR).

## Step 2: Push Your Container Image to ECR

Lambda requires container images to be in ECR. Push your image:

```bash
ECR_URL=$(terraform output -raw ecr_repository_url)
AWS_REGION=$(terraform output -raw region 2>/dev/null || echo "us-east-1")

# Login to ECR
aws ecr get-login-password --region $AWS_REGION | docker login --username AWS --password-stdin $ECR_URL

# Tag and push your image
docker tag alpine:latest $ECR_URL:alpine-latest
docker push $ECR_URL:alpine-latest
```

## Step 3: Export the Backend Configuration

```bash
# Quick method
terraform output -raw backend_env

# Or manually
export AWS_REGION=$(terraform output -raw region 2>/dev/null || echo "us-east-1")
export SOCKERLESS_LAMBDA_ROLE_ARN=$(terraform output -raw execution_role_arn)
export SOCKERLESS_LAMBDA_MEMORY_SIZE=1024
export SOCKERLESS_LAMBDA_TIMEOUT=900
export SOCKERLESS_CALLBACK_URL=http://<YOUR_BACKEND_HOST>:9100
```

**Important:** `SOCKERLESS_CALLBACK_URL` is required. Lambda uses reverse agent mode exclusively — the function cannot accept inbound connections. Replace `<YOUR_BACKEND_HOST>` with your machine's IP/hostname that is reachable from the Lambda VPC (or use a public endpoint).

## Step 4: Build and Run the Backend

```bash
cd backends/lambda
go build -o sockerless-backend-lambda ./cmd/sockerless-backend-lambda
./sockerless-backend-lambda -addr :9100
```

## Step 5: Configure Docker to Use Sockerless

```bash
cd frontends/docker
go build -o sockerless-frontend-docker .
./sockerless-frontend-docker -backend http://localhost:9100 -addr unix:///tmp/sockerless.sock

export DOCKER_HOST=unix:///tmp/sockerless.sock
```

## Step 6: Use Docker Commands

### Pull an image

```bash
# Reference must match an ECR image URI
ECR_URL=$(cd backends/lambda/examples/terraform && terraform output -raw ecr_repository_url)
docker pull $ECR_URL:alpine-latest
```

### Run a container

```bash
# Run a command (creates Lambda function, invokes it)
docker run --rm $ECR_URL:alpine-latest echo "Hello from Lambda!"
```

Behind the scenes:
1. `docker create` → `Lambda.CreateFunction` (container image package type)
2. `docker start` → `Lambda.Invoke` (async)
3. Agent inside function calls back to `SOCKERLESS_CALLBACK_URL`
4. `docker rm` → `Lambda.DeleteFunction`

### Create, exec, and inspect

```bash
# Create a container
docker create --name myfunc $ECR_URL:alpine-latest tail -f /dev/null

# Start it (invokes the function, agent calls back)
docker start myfunc

# Execute commands via reverse agent
docker exec myfunc ls /
docker exec myfunc cat /etc/os-release

# View logs (from CloudWatch)
docker logs myfunc

# Inspect
docker inspect myfunc

# Remove (deletes the Lambda function)
docker rm -f myfunc
```

### Limitations to be aware of

```bash
# Stop is a no-op (Lambda runs to completion)
docker stop myfunc    # returns 204 but does nothing

# Kill only disconnects the reverse agent
docker kill myfunc

# No follow mode for logs
docker logs -f myfunc  # returns a single snapshot, doesn't stream

# No bind mounts or volumes
docker run -v /data:/data ...  # volumes are not supported
```

## Step 7: Destroy the Infrastructure

```bash
cd backends/lambda/examples/terraform
terraform destroy
```

**Important:** Delete any Lambda functions created by Sockerless first:

```bash
# List functions created by Sockerless (named skls-*)
aws lambda list-functions --query 'Functions[?starts_with(FunctionName, `skls-`)].FunctionName' --output text

# Delete them
aws lambda delete-function --function-name skls-<id>
```

## Architecture Diagram

```
┌──────────────┐     ┌──────────────────┐     ┌────────────────────────┐
│  docker CLI  │────▶│ Sockerless       │────▶│ AWS Lambda             │
│              │     │ Frontend + Backend│     │                        │
│ pull, create,│     │ (localhost:9100)  │     │ CreateFunction         │
│ start, exec, │     │                  │◀────│ Invoke (async)         │
│ logs, rm     │     │ ◀── agent calls  │     │ DeleteFunction         │
└──────────────┘     │     back here    │     │ GetLogEvents           │
                     └──────────────────┘     └────────────────────────┘
                            ▲
                            │ reverse agent
                            │ WebSocket callback
```

## Key Differences from Vanilla Docker

| Feature | Vanilla Docker | Lambda Backend |
|---------|---------------|----------------|
| Stop | Sends SIGTERM | No-op (runs to completion) |
| Kill | Sends signal | Disconnects agent only |
| Logs follow | Real-time stream | Single snapshot |
| Bind mounts | Host paths | Not supported |
| Volumes | Docker volumes | Not supported |
| Networks | Docker networks | Not supported (optional VPC) |
| Exec | nsenter | Reverse agent relay |
| Port bindings | Host port mapping | Not supported |

## Estimated Costs

- **Lambda**: Pay-per-invocation ($0.20/1M requests) + compute time ($0.0000166667/GB-second)
- **CloudWatch Logs**: Per GB ingested ($0.50/GB)
- **ECR**: Per GB stored ($0.10/GB/month)

Lambda is extremely cost-effective for short-lived containers. No idle costs.

## Troubleshooting

**Function creation fails:** Ensure the execution role ARN is correct and has ECR pull + CloudWatch Logs permissions.

**Agent callback timeout:** The backend must be reachable from the Lambda function. If using VPC-mode Lambda, ensure the subnets have a NAT Gateway for outbound. If not using VPC mode, the callback URL must be publicly accessible.

**No logs:** CloudWatch log group is auto-created by Lambda as `/aws/lambda/{functionName}`. It may take a few seconds after invocation.

**Image not found:** Lambda only supports ECR images. Ensure the image is pushed to the ECR repository before creating a container.
