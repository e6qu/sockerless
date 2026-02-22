# Sockerless Terraform Infrastructure

Terraform infrastructure-as-code for provisioning Sockerless serverless and container backends across AWS, GCP, and Azure, managed with [Terragrunt](https://terragrunt.gruntwork.io/).

## Directory Structure

```
terraform/
  terragrunt.hcl      # Root Terragrunt config (inherited by all environments)
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
| ecs/simulator          | ecs      | AWS (LocalStack) | localhost:4566 |
| cloudrun/live          | cloudrun | GCP            | --             |
| cloudrun/simulator     | cloudrun | GCP (simulator)  | localhost:4567 |
| aca/live               | aca      | Azure          | --             |
| aca/simulator          | aca      | Azure (simulator)| localhost:4568 |
| lambda/live            | lambda   | AWS            | --             |
| lambda/simulator       | lambda   | AWS (LocalStack) | localhost:4566 |
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
- For simulator environments:
  - [LocalStack](https://localstack.cloud/) for AWS simulation (port 4566)
  - GCP simulator (port 4567)
  - Azure simulator (port 4568)

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
# Start the simulator first (e.g., LocalStack for AWS backends)
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
