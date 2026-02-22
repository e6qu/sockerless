# Sockerless Cloud Deployment Guide

How to deploy Sockerless to real AWS, GCP, and Azure infrastructure using the terraform modules that have been validated against local simulators.

---

## Table of Contents

- [Architecture Overview](#architecture-overview)
- [Building Binaries](#building-binaries)
- [AWS (ECS + Lambda)](#aws-ecs--lambda)
- [GCP (Cloud Run + Cloud Functions)](#gcp-cloud-run--cloud-functions)
- [Azure (Container Apps + Azure Functions)](#azure-container-apps--azure-functions)
- [Terraform Output → Environment Variable Reference](#terraform-output--environment-variable-reference)
- [Validation](#validation)
- [Tear Down](#tear-down)
- [CI/CD Integration](#cicd-integration)
- [Cost Estimates](#cost-estimates)

---

## Architecture Overview

```
Docker Client (SDK / CLI / CI Runner)
        │
        ▼
┌─────────────────────────────┐
│  Frontend                   │  binary: sockerless-frontend-docker
│  Docker REST API v1.44      │  listens on :2375 (TCP) or unix socket
│  Stateless — translates     │
│  Docker API → internal API  │
└──────────┬──────────────────┘
           │  HTTP/JSON (/internal/v1/...)
           ▼
┌─────────────────────────────┐
│  Backend                    │  binary: sockerless-backend-{ecs,lambda,...}
│  Cloud-specific             │  listens on :9100
│  Creates real cloud tasks   │
└──────────┬──────────────────┘
           │  Cloud SDK calls
           ▼
┌─────────────────────────────┐
│  Cloud Workload             │  ECS task / Cloud Run job / Container App /
│  ┌───────┐  ┌────────────┐  │  Lambda function / Cloud Function / Azure Function
│  │ Agent │←→│ User Image │  │
│  │ :9111 │  │            │  │
│  └───────┘  └────────────┘  │
└─────────────────────────────┘
```

**Three binaries:**

| Binary | Purpose |
|--------|---------|
| `sockerless-frontend-docker` | Stateless Docker API proxy. Translates Docker REST API v1.44 into internal API calls to the backend. |
| `sockerless-backend-{name}` | Cloud-specific backend. Creates/manages cloud workloads (ECS tasks, Cloud Run jobs, etc.). One of: `ecs`, `lambda`, `cloudrun`, `gcf`, `aca`, `azf`. |
| `sockerless-agent` | Runs inside each cloud workload as a sidecar (containers) or baked into the function image (FaaS). Provides exec/attach over WebSocket. |

**Container backends (ECS, Cloud Run, ACA):** The agent runs as a sidecar container alongside the user's image. The backend injects it automatically via `SOCKERLESS_AGENT_IMAGE`.

**FaaS backends (Lambda, GCF, AZF):** The agent is baked into the function image. It uses reverse WebSocket connections (`--callback` mode) to dial out to the backend since inbound connections aren't possible.

**Frontend + backend run locally** (or on a VM/bastion host with network access to the cloud). Only the agent runs inside cloud workloads.

---

## Building Binaries

All binaries are built from the project root using `go build`:

```bash
# Frontend
go build -o sockerless-frontend-docker ./frontends/docker/cmd

# Container backends
go build -o sockerless-backend-ecs       ./backends/ecs/cmd/sockerless-backend-ecs
go build -o sockerless-backend-cloudrun  ./backends/cloudrun/cmd/sockerless-backend-cloudrun
go build -o sockerless-backend-aca       ./backends/aca/cmd/sockerless-backend-aca

# FaaS backends
go build -o sockerless-backend-lambda    ./backends/lambda/cmd/sockerless-backend-lambda
go build -o sockerless-backend-gcf       ./backends/cloudrun-functions/cmd/sockerless-backend-gcf
go build -o sockerless-backend-azf       ./backends/azure-functions/cmd/sockerless-backend-azf

# Agent
go build -o sockerless-agent             ./agent/cmd/sockerless-agent
```

**Cross-compile for Linux** (if building on macOS for cloud deployment):

```bash
GOOS=linux GOARCH=amd64 go build -o sockerless-agent ./agent/cmd/sockerless-agent
```

### Agent Container Image

For container backends, the agent must be available as a container image. Build and tag it:

```dockerfile
# Dockerfile.agent
FROM alpine:latest
COPY sockerless-agent /usr/local/bin/sockerless-agent
ENTRYPOINT ["/usr/local/bin/sockerless-agent"]
```

```bash
GOOS=linux GOARCH=amd64 go build -o sockerless-agent ./agent/cmd/sockerless-agent
docker build -f Dockerfile.agent -t sockerless/agent:latest .
```

For FaaS backends, the agent is baked into the function image (see per-cloud sections below).

---

## AWS (ECS + Lambda)

### Prerequisites

- AWS account with appropriate permissions (ECS, ECR, VPC, IAM, CloudWatch, EFS, Lambda)
- `aws` CLI configured (`aws configure` or environment variables)
- Terraform >= 1.5 and Terragrunt >= 0.50
- Docker (for building and pushing images)

### State Backend Bootstrap (one-time)

Create the S3 bucket and DynamoDB table for terraform remote state:

```bash
aws s3api create-bucket \
    --bucket sockerless-terraform-state \
    --region us-east-1

aws s3api put-bucket-versioning \
    --bucket sockerless-terraform-state \
    --versioning-configuration Status=Enabled

aws dynamodb create-table \
    --table-name sockerless-terraform-locks \
    --attribute-definitions AttributeName=LockID,AttributeType=S \
    --key-schema AttributeName=LockID,KeyType=HASH \
    --billing-mode PAY_PER_REQUEST \
    --region us-east-1
```

### ECS Deployment

**1. Apply terraform:**

```bash
cd terraform
make init-ecs-live
make apply-ecs-live
```

This creates ~21 resources: VPC (2 AZs, public + private subnets), NAT gateway, ECS Fargate cluster, EFS filesystem, ECR repository, IAM roles (task + execution), CloudWatch log group, Cloud Map namespace, security groups.

**2. Build and push agent image to ECR:**

```bash
# Get ECR repository URL from terraform output
ECR_URL=$(cd terraform/environments/ecs/live && terragrunt output -raw ecr_repository_url)

# Build agent for linux/amd64
GOOS=linux GOARCH=amd64 go build -o sockerless-agent ./agent/cmd/sockerless-agent

# Build and push Docker image
docker build -f Dockerfile.agent -t "$ECR_URL:latest" .
aws ecr get-login-password --region us-east-1 | docker login --username AWS --password-stdin "$ECR_URL"
docker push "$ECR_URL:latest"
```

**3. Extract outputs and configure environment:**

```bash
cd terraform/environments/ecs/live
export SOCKERLESS_ECS_CLUSTER="$(terragrunt output -raw ecs_cluster_name)"
export SOCKERLESS_ECS_SUBNETS="$(terragrunt output -json private_subnet_ids | jq -r 'join(",")')"
export SOCKERLESS_ECS_SECURITY_GROUPS="$(terragrunt output -raw task_security_group_id)"
export SOCKERLESS_ECS_TASK_ROLE_ARN="$(terragrunt output -raw task_role_arn)"
export SOCKERLESS_ECS_EXECUTION_ROLE_ARN="$(terragrunt output -raw execution_role_arn)"
export SOCKERLESS_ECS_LOG_GROUP="$(terragrunt output -raw log_group_name)"
export SOCKERLESS_AGENT_EFS_ID="$(terragrunt output -raw efs_filesystem_id)"
export SOCKERLESS_AGENT_IMAGE="$ECR_URL:latest"
```

**4. Start backend and frontend:**

```bash
./sockerless-backend-ecs --addr :9100 &
./sockerless-frontend-docker --addr :2375 --backend http://localhost:9100 &
```

**5. Validate:**

```bash
DOCKER_HOST=tcp://localhost:2375 docker run --rm alpine echo "hello from ECS"
```

### Lambda Deployment

**1. Apply terraform:**

```bash
cd terraform
make init-lambda-live
make apply-lambda-live
```

This creates ~5 resources: IAM execution role, ECR repository, CloudWatch log group.

**2. Build and push function image to ECR:**

Lambda functions run as container images. The image must include both the agent binary (for reverse exec) and whatever runtime your workloads need.

```bash
ECR_URL=$(cd terraform/environments/lambda/live && terragrunt output -raw ecr_repository_url)

aws ecr get-login-password --region us-east-1 | docker login --username AWS --password-stdin "$ECR_URL"
docker push "$ECR_URL:latest"
```

**3. Extract outputs and configure environment:**

```bash
cd terraform/environments/lambda/live
export SOCKERLESS_LAMBDA_ROLE_ARN="$(terragrunt output -raw execution_role_arn)"
export SOCKERLESS_LAMBDA_LOG_GROUP="$(terragrunt output -raw log_group_name)"
export SOCKERLESS_CALLBACK_URL="http://<backend-host>:9100"  # Must be reachable from Lambda
```

> **Note:** `SOCKERLESS_CALLBACK_URL` must be a URL that Lambda functions can reach. If the backend runs on a local machine, you'll need a VPN, bastion, or public endpoint with auth. Lambda functions use reverse WebSocket connections to dial out to this URL.

**4. Start backend and frontend:**

```bash
./sockerless-backend-lambda --addr :9100 &
./sockerless-frontend-docker --addr :2375 --backend http://localhost:9100 &
```

> **Timeout limit:** Lambda has a maximum execution timeout of 15 minutes (900 seconds). Configurable via `SOCKERLESS_LAMBDA_TIMEOUT` (default: 900).

---

## GCP (Cloud Run + Cloud Functions)

### Prerequisites

- GCP project with billing enabled
- `gcloud` CLI authenticated (`gcloud auth login` + `gcloud auth application-default login`)
- Terraform >= 1.5 and Terragrunt >= 0.50
- Docker (for building and pushing images)

### State Backend Bootstrap (one-time)

Create the GCS bucket for terraform remote state:

```bash
gcloud storage buckets create gs://sockerless-terraform-state \
    --project=sockerless \
    --location=us \
    --uniform-bucket-level-access

gcloud storage buckets update gs://sockerless-terraform-state --versioning
```

### Cloud Run Deployment

**1. Apply terraform:**

```bash
cd terraform
make init-cloudrun-live
make apply-cloudrun-live
```

This creates ~13 resources: enabled APIs, VPC network, Serverless VPC Access connector, Cloud DNS private zone, GCS bucket, Artifact Registry repository, IAM service account + bindings.

**2. Build and push agent image to Artifact Registry:**

```bash
AR_URL=$(cd terraform/environments/cloudrun/live && terragrunt output -raw artifact_registry_repository_url)
REGION=$(cd terraform/environments/cloudrun/live && terragrunt output -raw region)

# Configure Docker for Artifact Registry
gcloud auth configure-docker "${REGION}-docker.pkg.dev"

# Build and push
GOOS=linux GOARCH=amd64 go build -o sockerless-agent ./agent/cmd/sockerless-agent
docker build -f Dockerfile.agent -t "$AR_URL/agent:latest" .
docker push "$AR_URL/agent:latest"
```

**3. Extract outputs and configure environment:**

```bash
cd terraform/environments/cloudrun/live
export SOCKERLESS_GCR_PROJECT="$(terragrunt output -raw project_id)"
export SOCKERLESS_GCR_REGION="$(terragrunt output -raw region)"
export SOCKERLESS_GCR_VPC_CONNECTOR="$(terragrunt output -raw vpc_connector_name)"
export SOCKERLESS_GCR_AGENT_IMAGE="$AR_URL/agent:latest"
```

**4. Start backend and frontend:**

```bash
./sockerless-backend-cloudrun --addr :9100 &
./sockerless-frontend-docker --addr :2375 --backend http://localhost:9100 &
```

**5. Validate:**

```bash
DOCKER_HOST=tcp://localhost:2375 docker run --rm alpine echo "hello from Cloud Run"
```

### Cloud Functions Deployment

**1. Apply terraform:**

```bash
cd terraform
make init-gcf-live
make apply-gcf-live
```

This creates ~7 resources: enabled APIs, Artifact Registry repository, IAM service account + bindings.

**2. Build and push function image:**

```bash
AR_URL=$(cd terraform/environments/gcf/live && terragrunt output -raw artifact_registry_repository_url)
REGION=$(cd terraform/environments/gcf/live && terragrunt output -raw region)

gcloud auth configure-docker "${REGION}-docker.pkg.dev"
docker push "$AR_URL/function:latest"
```

**3. Extract outputs and configure environment:**

```bash
cd terraform/environments/gcf/live
export SOCKERLESS_GCF_PROJECT="$(terragrunt output -raw project_id)"
export SOCKERLESS_GCF_REGION="$(terragrunt output -raw region)"
export SOCKERLESS_GCF_SERVICE_ACCOUNT="$(terragrunt output -raw service_account_email)"
export SOCKERLESS_CALLBACK_URL="http://<backend-host>:9100"  # Must be reachable from GCF
```

**4. Start backend and frontend:**

```bash
./sockerless-backend-gcf --addr :9100 &
./sockerless-frontend-docker --addr :2375 --backend http://localhost:9100 &
```

> **Timeout limit:** GCF 2nd gen has a maximum execution timeout of 60 minutes (3600 seconds). Configurable via `SOCKERLESS_GCF_TIMEOUT` (default: 3600).

---

## Azure (Container Apps + Azure Functions)

### Prerequisites

- Azure subscription
- `az` CLI authenticated (`az login`)
- Terraform >= 1.5 and Terragrunt >= 0.50
- Docker (for building and pushing images)

### State Backend Bootstrap (one-time)

Create the resource group, storage account, and blob container for terraform remote state:

```bash
az group create \
    --name sockerless-terraform-state \
    --location eastus

az storage account create \
    --name sockerlesstfstate \
    --resource-group sockerless-terraform-state \
    --location eastus \
    --sku Standard_LRS \
    --kind StorageV2

az storage container create \
    --name tfstate \
    --account-name sockerlesstfstate
```

### Container Apps Deployment

**1. Apply terraform:**

```bash
cd terraform
make init-aca-live
make apply-aca-live
```

This creates ~18 resources: resource group, VNet + subnet + NSG, Log Analytics workspace, Container Apps environment, storage account + file share, Azure Container Registry, user-assigned managed identity + RBAC role assignments, private DNS zone.

**2. Build and push agent image to ACR:**

```bash
ACR_SERVER=$(cd terraform/environments/aca/live && terragrunt output -raw acr_login_server)

# Login to ACR
az acr login --name "${ACR_SERVER%%.*}"

# Build and push
GOOS=linux GOARCH=amd64 go build -o sockerless-agent ./agent/cmd/sockerless-agent
docker build -f Dockerfile.agent -t "$ACR_SERVER/agent:latest" .
docker push "$ACR_SERVER/agent:latest"
```

**3. Extract outputs and configure environment:**

```bash
cd terraform/environments/aca/live
export SOCKERLESS_ACA_SUBSCRIPTION_ID="$(az account show --query id -o tsv)"
export SOCKERLESS_ACA_RESOURCE_GROUP="$(terragrunt output -raw resource_group_name)"
export SOCKERLESS_ACA_ENVIRONMENT="$(terragrunt output -raw managed_environment_name)"
export SOCKERLESS_ACA_LOCATION="$(terragrunt output -raw location)"
export SOCKERLESS_ACA_LOG_ANALYTICS_WORKSPACE="$(terragrunt output -raw log_analytics_workspace_name)"
export SOCKERLESS_ACA_STORAGE_ACCOUNT="$(terragrunt output -raw storage_account_name)"
export SOCKERLESS_ACA_AGENT_IMAGE="$ACR_SERVER/agent:latest"
```

**4. Start backend and frontend:**

```bash
./sockerless-backend-aca --addr :9100 &
./sockerless-frontend-docker --addr :2375 --backend http://localhost:9100 &
```

**5. Validate:**

```bash
DOCKER_HOST=tcp://localhost:2375 docker run --rm alpine echo "hello from Container Apps"
```

### Azure Functions Deployment

**1. Apply terraform:**

```bash
cd terraform
make init-azf-live
make apply-azf-live
```

This creates ~11 resources: resource group, storage account, App Service Plan, Azure Container Registry, user-assigned managed identity + RBAC role assignments, Log Analytics workspace, Application Insights.

**2. Build and push function image to ACR:**

```bash
ACR_SERVER=$(cd terraform/environments/azf/live && terragrunt output -raw acr_login_server)

az acr login --name "${ACR_SERVER%%.*}"
docker push "$ACR_SERVER/function:latest"
```

**3. Extract outputs and configure environment:**

```bash
cd terraform/environments/azf/live
export SOCKERLESS_AZF_SUBSCRIPTION_ID="$(az account show --query id -o tsv)"
export SOCKERLESS_AZF_RESOURCE_GROUP="$(terragrunt output -raw resource_group_name)"
export SOCKERLESS_AZF_LOCATION="$(terragrunt output -raw location)"
export SOCKERLESS_AZF_STORAGE_ACCOUNT="$(terragrunt output -raw storage_account_name)"
export SOCKERLESS_AZF_REGISTRY="$(terragrunt output -raw acr_login_server)"
export SOCKERLESS_AZF_APP_SERVICE_PLAN="$(terragrunt output -raw app_service_plan_id)"
export SOCKERLESS_AZF_LOG_ANALYTICS_WORKSPACE="$(terragrunt output -raw log_analytics_workspace_id)"
export SOCKERLESS_CALLBACK_URL="http://<backend-host>:9100"  # Must be reachable from AZF
```

**4. Start backend and frontend:**

```bash
./sockerless-backend-azf --addr :9100 &
./sockerless-frontend-docker --addr :2375 --backend http://localhost:9100 &
```

> **Timeout limit:** Consumption plan (Y1) has a 10-minute maximum. Premium plan (EP1) supports up to 60 minutes. Configurable via `SOCKERLESS_AZF_TIMEOUT` (default: 600).

---

## Terraform Output → Environment Variable Reference

Complete mapping from terraform outputs to backend environment variables, proven by `tests/terraform-integration/run-test.sh`.

### ECS

| Terraform Output | Environment Variable | Required |
|---|---|:---:|
| `ecs_cluster_name` | `SOCKERLESS_ECS_CLUSTER` | Yes |
| `private_subnet_ids` | `SOCKERLESS_ECS_SUBNETS` (comma-separated) | Yes |
| `task_security_group_id` | `SOCKERLESS_ECS_SECURITY_GROUPS` | No |
| `task_role_arn` | `SOCKERLESS_ECS_TASK_ROLE_ARN` | No |
| `execution_role_arn` | `SOCKERLESS_ECS_EXECUTION_ROLE_ARN` | Yes |
| `log_group_name` | `SOCKERLESS_ECS_LOG_GROUP` | No (default: `/sockerless`) |
| `efs_filesystem_id` | `SOCKERLESS_AGENT_EFS_ID` | No |
| `ecr_repository_url` | `SOCKERLESS_AGENT_IMAGE` (after push) | No (default: `sockerless/agent:latest`) |
| — | `AWS_REGION` | No (default: `us-east-1`) |
| — | `SOCKERLESS_ECS_PUBLIC_IP` | No (default: `false`) |

### Lambda

| Terraform Output | Environment Variable | Required |
|---|---|:---:|
| `execution_role_arn` | `SOCKERLESS_LAMBDA_ROLE_ARN` | Yes |
| `log_group_name` | `SOCKERLESS_LAMBDA_LOG_GROUP` | No (default: `/sockerless/lambda`) |
| `ecr_repository_url` | (for image push) | — |
| — | `AWS_REGION` | No (default: `us-east-1`) |
| — | `SOCKERLESS_CALLBACK_URL` | Yes (for reverse exec) |
| — | `SOCKERLESS_LAMBDA_MEMORY_SIZE` | No (default: `1024` MB) |
| — | `SOCKERLESS_LAMBDA_TIMEOUT` | No (default: `900` seconds) |
| — | `SOCKERLESS_LAMBDA_SUBNETS` | No (for VPC-attached functions) |
| — | `SOCKERLESS_LAMBDA_SECURITY_GROUPS` | No (for VPC-attached functions) |

### Cloud Run

| Terraform Output | Environment Variable | Required |
|---|---|:---:|
| `project_id` | `SOCKERLESS_GCR_PROJECT` | Yes |
| `region` | `SOCKERLESS_GCR_REGION` | No (default: `us-central1`) |
| `vpc_connector_name` | `SOCKERLESS_GCR_VPC_CONNECTOR` | No |
| `artifact_registry_repository_url` | `SOCKERLESS_GCR_AGENT_IMAGE` (after push) | No (default: `sockerless/agent:latest`) |
| — | `SOCKERLESS_GCR_LOG_ID` | No (default: `sockerless`) |

### Cloud Functions (GCF)

| Terraform Output | Environment Variable | Required |
|---|---|:---:|
| `project_id` | `SOCKERLESS_GCF_PROJECT` | Yes |
| `region` | `SOCKERLESS_GCF_REGION` | No (default: `us-central1`) |
| `service_account_email` | `SOCKERLESS_GCF_SERVICE_ACCOUNT` | No |
| `artifact_registry_repository_url` | (for image push) | — |
| — | `SOCKERLESS_CALLBACK_URL` | Yes (for reverse exec) |
| — | `SOCKERLESS_GCF_TIMEOUT` | No (default: `3600` seconds) |
| — | `SOCKERLESS_GCF_MEMORY` | No (default: `1Gi`) |
| — | `SOCKERLESS_GCF_CPU` | No (default: `1`) |

### Container Apps (ACA)

| Terraform Output | Environment Variable | Required |
|---|---|:---:|
| `resource_group_name` | `SOCKERLESS_ACA_RESOURCE_GROUP` | Yes |
| `managed_environment_name` | `SOCKERLESS_ACA_ENVIRONMENT` | No (default: `sockerless`) |
| `location` | `SOCKERLESS_ACA_LOCATION` | No (default: `eastus`) |
| `log_analytics_workspace_name` | `SOCKERLESS_ACA_LOG_ANALYTICS_WORKSPACE` | No |
| `storage_account_name` | `SOCKERLESS_ACA_STORAGE_ACCOUNT` | No |
| `acr_login_server` | `SOCKERLESS_ACA_AGENT_IMAGE` (after push) | No (default: `sockerless/agent:latest`) |
| — | `SOCKERLESS_ACA_SUBSCRIPTION_ID` | Yes |

### Azure Functions (AZF)

| Terraform Output | Environment Variable | Required |
|---|---|:---:|
| `resource_group_name` | `SOCKERLESS_AZF_RESOURCE_GROUP` | Yes |
| `location` | `SOCKERLESS_AZF_LOCATION` | No (default: `eastus`) |
| `storage_account_name` | `SOCKERLESS_AZF_STORAGE_ACCOUNT` | Yes |
| `acr_login_server` | `SOCKERLESS_AZF_REGISTRY` | No |
| `app_service_plan_id` | `SOCKERLESS_AZF_APP_SERVICE_PLAN` | No |
| `log_analytics_workspace_id` | `SOCKERLESS_AZF_LOG_ANALYTICS_WORKSPACE` | No |
| — | `SOCKERLESS_AZF_SUBSCRIPTION_ID` | Yes |
| — | `SOCKERLESS_CALLBACK_URL` | Yes (for reverse exec) |
| — | `SOCKERLESS_AZF_TIMEOUT` | No (default: `600` seconds) |

### Helper Script

Extract all outputs for a backend at once:

```bash
# From the backend's live terragrunt directory:
cd terraform/environments/<backend>/live
terragrunt output -json | jq -r 'to_entries[] | "export \(.key | ascii_upcase)=\"\(.value.value)\""'
```

---

## Validation

### Quick Validation

Run a single container through the full stack:

```bash
export DOCKER_HOST=tcp://localhost:2375
docker run --rm alpine echo "hello from sockerless"
```

### Full Validation with act

Run the smoke test workflow (a minimal GitHub Actions workflow) against the deployed backend:

```bash
export DOCKER_HOST=tcp://localhost:2375
act push \
    --workflows smoke-tests/act/workflows/ \
    -P ubuntu-latest=alpine:latest \
    --container-daemon-socket tcp://localhost:2375
```

### Backend Health Check

Each backend exposes an info endpoint:

```bash
curl http://localhost:9100/internal/v1/info
```

### Frontend Health Check

The frontend responds to Docker's ping:

```bash
curl http://localhost:2375/_ping
```

---

## Tear Down

### Destroy Cloud Resources

```bash
cd terraform
make destroy-ecs-live
make destroy-lambda-live
make destroy-cloudrun-live
make destroy-gcf-live
make destroy-aca-live
make destroy-azf-live
```

### Clean Up State Backend (manual)

State backend resources are **not** managed by terraform (they store the state itself). Remove them manually when no longer needed:

**AWS:**
```bash
# Empty and delete the S3 bucket
aws s3 rb s3://sockerless-terraform-state --force
aws dynamodb delete-table --table-name sockerless-terraform-locks --region us-east-1
```

**GCP:**
```bash
gcloud storage rm -r gs://sockerless-terraform-state
```

**Azure:**
```bash
az group delete --name sockerless-terraform-state --yes
```

---

## CI/CD Integration

### GitHub Actions Deployment

```yaml
# .github/workflows/deploy.yml
name: Deploy
on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        backend: [ecs, lambda, cloudrun, gcf, aca, azf]
    steps:
      - uses: actions/checkout@v4

      - uses: hashicorp/setup-terraform@v3
        with:
          terraform_version: "1.8.5"

      - name: Install Terragrunt
        run: |
          curl -fsSL "https://github.com/gruntwork-io/terragrunt/releases/download/v0.77.22/terragrunt_linux_amd64" \
            -o /usr/local/bin/terragrunt
          chmod +x /usr/local/bin/terragrunt

      # Configure cloud credentials (pick one per matrix entry)
      - uses: aws-actions/configure-aws-credentials@v4
        if: contains(fromJSON('["ecs","lambda"]'), matrix.backend)
        with:
          aws-access-key-id: ${{ secrets.AWS_ACCESS_KEY_ID }}
          aws-secret-access-key: ${{ secrets.AWS_SECRET_ACCESS_KEY }}
          aws-region: us-east-1

      - uses: google-github-actions/auth@v2
        if: contains(fromJSON('["cloudrun","gcf"]'), matrix.backend)
        with:
          credentials_json: ${{ secrets.GCP_CREDENTIALS }}

      - uses: azure/login@v2
        if: contains(fromJSON('["aca","azf"]'), matrix.backend)
        with:
          creds: ${{ secrets.AZURE_CREDENTIALS }}

      - name: Terraform Apply
        run: |
          cd terraform
          make init-${{ matrix.backend }}-live
          make apply-${{ matrix.backend }}-live
```

### GitLab CI Deployment

```yaml
# .gitlab-ci.yml (deployment additions)
.deploy-base:
  image: golang:1.24
  before_script:
    - apt-get update && apt-get install -y curl unzip jq
    - curl -fsSL https://releases.hashicorp.com/terraform/1.8.5/terraform_1.8.5_linux_amd64.zip -o /tmp/tf.zip
      && unzip /tmp/tf.zip -d /usr/local/bin/
    - curl -fsSL https://github.com/gruntwork-io/terragrunt/releases/download/v0.77.22/terragrunt_linux_amd64
      -o /usr/local/bin/terragrunt && chmod +x /usr/local/bin/terragrunt
  when: manual

deploy-ecs:
  extends: .deploy-base
  script:
    - cd terraform && make init-ecs-live && make apply-ecs-live
  variables:
    AWS_ACCESS_KEY_ID: $AWS_ACCESS_KEY_ID
    AWS_SECRET_ACCESS_KEY: $AWS_SECRET_ACCESS_KEY
    AWS_DEFAULT_REGION: us-east-1

deploy-cloudrun:
  extends: .deploy-base
  script:
    - cd terraform && make init-cloudrun-live && make apply-cloudrun-live
  variables:
    GOOGLE_APPLICATION_CREDENTIALS: $GCP_KEY_FILE

deploy-aca:
  extends: .deploy-base
  script:
    - cd terraform && make init-aca-live && make apply-aca-live
  variables:
    ARM_CLIENT_ID: $ARM_CLIENT_ID
    ARM_CLIENT_SECRET: $ARM_CLIENT_SECRET
    ARM_TENANT_ID: $ARM_TENANT_ID
    ARM_SUBSCRIPTION_ID: $ARM_SUBSCRIPTION_ID
```

### Terraform Plan on PR, Apply on Merge

A common pattern is to run `terragrunt plan` on pull requests and `terragrunt apply` on merge to main. Use the `plan-%` and `apply-%` Makefile targets:

```bash
# PR check
cd terraform && make plan-ecs-live

# On merge
cd terraform && make apply-ecs-live
```

---

## Cost Estimates

Rough monthly cost estimates when idle (no workloads running). Actual costs depend on usage.

### AWS

| Resource | ECS | Lambda | Notes |
|----------|----:|-------:|-------|
| NAT Gateway | $32 | — | $0.045/hr + $0.045/GB processed |
| EFS | <$1 | — | $0.30/GB-month (standard), minimal with no data |
| ECR | <$1 | <$1 | $0.10/GB-month for stored images |
| CloudWatch Logs | <$1 | <$1 | $0.50/GB ingested |
| ECS Tasks | on-demand | — | Fargate pricing per vCPU-hr + GB-hr |
| Lambda | — | on-demand | $0.20/1M requests + compute time |
| **Idle total** | **~$33** | **~$1** | |

> **Note:** The NAT Gateway is the dominant fixed cost for ECS. Consider NAT instances or VPC endpoints to reduce costs in non-production environments.

### GCP

| Resource | Cloud Run | GCF | Notes |
|----------|----------:|----:|-------|
| VPC Connector | $7 | — | e2-micro minimum 2 instances ($0.0042/hr each) |
| GCS | <$1 | — | $0.020/GB-month (standard) |
| Artifact Registry | <$1 | <$1 | $0.10/GB-month |
| Cloud Run Jobs | on-demand | — | Per vCPU-second + GB-second |
| Cloud Functions | — | on-demand | $0.40/1M invocations + compute |
| **Idle total** | **~$8** | **~$1** | |

### Azure

| Resource | ACA | AZF | Notes |
|----------|----:|----:|-------|
| Container Apps Environment | free | — | Consumption plan: no idle cost |
| ACR (Basic) | $5 | $5 | $0.167/day |
| Storage Account | <$1 | <$1 | LRS: $0.018/GB-month |
| Log Analytics | ~$1 | ~$1 | $2.76/GB ingested (free 5GB/month) |
| App Insights | — | <$1 | $2.76/GB ingested (free 5GB/month) |
| App Service Plan | — | free* | Y1 Consumption: pay per execution |
| **Idle total** | **~$7** | **~$7** | |

> \* Azure Functions Consumption plan (Y1) has no idle cost. Premium plan (EP1) starts at ~$150/month.
