# Using the AWS simulator with Terraform

## Prerequisites

- Terraform installed (`terraform version`)
- Simulator running on `http://localhost:4566`

## Provider configuration

Use the official `hashicorp/aws` provider with endpoint overrides pointing at the simulator:

```hcl
terraform {
  required_providers {
    aws = {
      source = "hashicorp/aws"
    }
  }
}

provider "aws" {
  region                      = "us-east-1"
  access_key                  = "test"
  secret_key                  = "test"
  skip_credentials_validation = true
  skip_metadata_api_check     = true
  skip_requesting_account_id  = true

  endpoints {
    ecs            = "http://localhost:4566"
    ecr            = "http://localhost:4566"
    lambda         = "http://localhost:4566"
    cloudwatchlogs = "http://localhost:4566"
    s3             = "http://localhost:4566/s3"
    iam            = "http://localhost:4566"
    sts            = "http://localhost:4566"
    ec2            = "http://localhost:4566"
    efs            = "http://localhost:4566"
    servicediscovery = "http://localhost:4566"
  }
}
```

The `skip_*` flags prevent the provider from making calls that the simulator doesn't need to handle (metadata API, credential validation, account ID lookup).

## Example resources

```hcl
data "aws_caller_identity" "current" {}

resource "aws_ecs_cluster" "main" {
  name = "my-cluster"
}

resource "aws_ecs_task_definition" "main" {
  family                = "my-task"
  container_definitions = jsonencode([{
    name      = "main"
    image     = "nginx:latest"
    essential = true
  }])
}

resource "aws_iam_role" "execution" {
  name               = "execution-role"
  assume_role_policy = jsonencode({
    Version   = "2012-10-17"
    Statement = [{
      Action    = "sts:AssumeRole"
      Effect    = "Allow"
      Principal = { Service = "ecs-tasks.amazonaws.com" }
    }]
  })
}
```

## Running

Pass the simulator endpoint via a variable or hardcode it:

```sh
# Using a variable
terraform init
terraform apply -auto-approve -var="endpoint=http://localhost:4566"
terraform destroy -auto-approve -var="endpoint=http://localhost:4566"
```

Or define the variable in a `variables.tf`:

```hcl
variable "endpoint" {
  description = "Simulator endpoint URL"
  type        = string
  default     = "http://localhost:4566"
}
```

Then reference `var.endpoint` in the provider endpoints block.

## Supported resources

The simulator supports the AWS API operations that these Terraform resources use:

| Category | Resources |
|----------|-----------|
| ECS | `aws_ecs_cluster`, `aws_ecs_task_definition`, `aws_ecs_service` |
| ECR | `aws_ecr_repository`, `aws_ecr_lifecycle_policy` |
| Lambda | `aws_lambda_function` |
| IAM | `aws_iam_role`, `aws_iam_role_policy`, `aws_iam_role_policy_attachment` |
| EC2 | `aws_vpc`, `aws_subnet`, `aws_internet_gateway`, `aws_nat_gateway`, `aws_route_table`, `aws_security_group` |
| S3 | `aws_s3_bucket`, `aws_s3_object` |
| EFS | `aws_efs_file_system`, `aws_efs_mount_target`, `aws_efs_access_point` |
| CloudWatch | `aws_cloudwatch_log_group` |
| Cloud Map | `aws_service_discovery_private_dns_namespace`, `aws_service_discovery_service` |

## Notes

- All state is in-memory and resets when the simulator restarts. Terraform state files will become stale after a restart.
- S3 endpoints use a `/s3` prefix (`http://localhost:4566/s3`).
- Authentication is not validated — any access key and secret will work.
