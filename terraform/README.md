# Sockerless Terraform Infrastructure

Terraform infrastructure-as-code for provisioning Sockerless serverless and container backends across AWS, GCP, and Azure, managed with [Terragrunt](https://terragrunt.gruntwork.io/).

## Directory Structure

```
terraform/
  terragrunt.hcl      # Root Terragrunt config (inherited by all environments)
  Makefile             # Make targets for init, plan, apply, destroy per environment
  modules/             # Reusable Terraform modules (one per backend type)
  environments/        # Terragrunt environment configurations
    <backend>/
      live/            # Provisions real cloud infrastructure
      simulator/       # Points at local cloud simulators
```

## Modules

| Module     | Cloud Provider | Description                          |
|------------|----------------|--------------------------------------|
| ecs        | AWS            | AWS ECS Fargate container backend    |
| cloudrun   | GCP            | Google Cloud Run container backend   |
| aca        | Azure          | Azure Container Apps backend         |
| lambda     | AWS            | AWS Lambda function backend          |
| gcf        | GCP            | Google Cloud Functions backend       |
| azf        | Azure          | Azure Functions backend              |

## Environments

Each backend has two environments:

| Environment | Purpose                                                                 |
|-------------|-------------------------------------------------------------------------|
| `live`      | Provisions real cloud infrastructure and runs integration tests against it |
| `simulator` | Points at local cloud simulators -- no real cloud resources needed        |

The full matrix of environments:

| Environment            | Module   | Cloud Provider | Simulator Port |
|------------------------|----------|----------------|----------------|
| ecs/live               | ecs      | AWS            | --             |
| ecs/simulator          | ecs      | AWS (simulator)  | localhost:4566 |
| cloudrun/live          | cloudrun | GCP            | --             |
| cloudrun/simulator     | cloudrun | GCP (simulator)  | localhost:4567 |
| aca/live               | aca      | Azure          | --             |
| aca/simulator          | aca      | Azure (simulator)| localhost:4568 |
| lambda/live            | lambda   | AWS            | --             |
| lambda/simulator       | lambda   | AWS (simulator)  | localhost:4566 |
| gcf/live               | gcf      | GCP            | --             |
| gcf/simulator          | gcf      | GCP (simulator)  | localhost:4567 |
| azf/live               | azf      | Azure          | --             |
| azf/simulator          | azf      | Azure (simulator)| localhost:4568 |

## Provider Version Constraints

| Provider  | Version   |
|-----------|-----------|
| Terraform | >= 1.5    |
| AWS       | ~> 5.0    |
| Google    | ~> 5.0    |
| AzureRM   | ~> 3.0    |

## State Backend Patterns

Each cloud provider uses its native remote state backend for `live` environments:

- **AWS (ecs, lambda):** S3 bucket with DynamoDB table for state locking.
- **GCP (cloudrun, gcf):** GCS bucket with built-in locking.
- **Azure (aca, azf):** Azure Blob Storage container with lease-based locking.

All `simulator` environments use **local** state since they do not interact with real cloud infrastructure.

State files are stored per-environment to ensure isolation.

## Tagging Conventions

All resources include the following tags/labels:

| Tag          | Description                              |
|--------------|------------------------------------------|
| project      | `sockerless`                             |
| environment  | Environment name (e.g., `live`)          |
| managed-by   | `terragrunt`                             |

## Prerequisites

- [Terraform](https://www.terraform.io/) >= 1.5
- [Terragrunt](https://terragrunt.gruntwork.io/) >= 0.50
- Cloud provider CLI tools (as needed):
  - `aws` CLI for ECS and Lambda backends
  - `gcloud` CLI for Cloud Run and GCF backends
  - `az` CLI for ACA and Azure Functions backends
- For simulator environments, each cloud has a built-in simulator in `simulators/`:
  - AWS simulator (port 4566) — `simulators/aws/`
  - GCP simulator (port 4567) — `simulators/gcp/`
  - Azure simulator (port 4568) — `simulators/azure/`

## Usage

All commands are run from the `terraform/` directory.

### Format and Lint

```bash
make fmt       # Auto-format all .tf files
make lint      # Check formatting (CI-friendly, non-destructive)
```

### Validate Modules

```bash
make validate  # Run terraform validate on all modules
```

### Deploy an Environment

```bash
make init-ecs-live       # Initialize the ecs live environment
make plan-ecs-live       # Preview changes
make apply-ecs-live      # Apply changes
```

### Run Against a Simulator

```bash
# Start the simulator first (e.g., simulators/aws for AWS backends)
make init-ecs-simulator
make plan-ecs-simulator
make apply-ecs-simulator
```

### Tear Down an Environment

```bash
make destroy-ecs-live
make destroy-ecs-simulator
```

### Bulk Operations

```bash
make plan-all    # Plan all environments via terragrunt run-all
make apply-all   # Apply all environments via terragrunt run-all
```

### Clean Up Local State

```bash
make clean    # Remove .terragrunt-cache, .terraform directories, and lock files
```

### List All Targets

```bash
make help
```

Valid backend names: `ecs`, `cloudrun`, `aca`, `lambda`, `gcf`, `azf`.
Valid environment names: `live`, `simulator`.

## State Backend Bootstrap

Remote state backends must be created once before the first `terragrunt init`. These resources are **not** managed by Terraform (they store the state itself).

### AWS (ECS, Lambda)

Create an S3 bucket and DynamoDB table:

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

### GCP (Cloud Run, GCF)

Create a GCS bucket:

```bash
gcloud storage buckets create gs://sockerless-terraform-state \
    --project=sockerless \
    --location=us \
    --uniform-bucket-level-access

gcloud storage buckets update gs://sockerless-terraform-state --versioning
```

### Azure (ACA, AZF)

Create a resource group, storage account, and blob container:

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

### Tear Down State Backends

Remove state backends manually when no longer needed:

```bash
# AWS
aws s3 rb s3://sockerless-terraform-state --force
aws dynamodb delete-table --table-name sockerless-terraform-locks --region us-east-1

# GCP
gcloud storage rm -r gs://sockerless-terraform-state

# Azure
az group delete --name sockerless-terraform-state --yes
```

## CI/CD Deployment

### GitHub Actions

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

### GitLab CI

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

### Plan on PR, Apply on Merge

A common pattern is to run `terragrunt plan` on pull requests and `terragrunt apply` on merge to main:

```bash
# PR check
cd terraform && make plan-ecs-live

# On merge
cd terraform && make apply-ecs-live
```
