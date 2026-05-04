# =============================================================================
# Lambda Terraform Module
# =============================================================================
#
# Provisions AWS infrastructure required by the Sockerless Lambda backend.
# This includes:
#   - IAM execution role with AWSLambdaBasicExecutionRole managed policy
#   - IAM policy for ECR image pull
#   - CloudWatch Logs log group
#   - ECR repository with image scanning and lifecycle policy
#
# The actual Lambda function creation and invocation happen at runtime in the
# Lambda backend (LAM-001). This module ensures the IAM role, ECR repo, and
# log group exist. The backend uses execution_role_arn and ecr_repository_url
# from the module outputs.
#
# Prerequisites:
#   - AWS provider configured with appropriate credentials
#   - Terraform >= 1.5
#
# Usage:
#   module "lambda" {
#     source      = "../../modules/lambda"
#     environment = "test"
#   }
# =============================================================================

locals {
  name_prefix = "${var.project_name}-${var.environment}"

  common_tags = merge(var.tags, {
    project     = var.project_name
    environment = var.environment
    component   = "lambda"
    managed-by  = "terraform"
  })
}

# =============================================================================
# Data Sources
# =============================================================================

data "aws_caller_identity" "current" {}

data "aws_partition" "current" {}

# =============================================================================
# Sockerless runtime sweep
# =============================================================================
#
# Sockerless creates Lambda functions at runtime (one per
# `docker create`/`docker run`); they're not in this module's terraform
# state. On `terragrunt destroy` we iterate every sockerless-managed
# function and (a) clear its VpcConfig so AWS Lambda starts releasing
# the hyperplane ENI, (b) delete the function. Without this sweep,
# orphan functions outlive the IAM execution role, and any subsequent
# ECS-stack destroy hangs on the SG/subnet because the ENIs are still
# attached. Equivalent of the ECS module's sockerless_runtime_sweep —
# kept symmetric across backends per the
# `[AWS teardown — terragrunt destroy must be self-sufficient]`
# project rule.
resource "null_resource" "sockerless_runtime_sweep" {
  triggers = {
    region = var.region
  }

  provisioner "local-exec" {
    when    = destroy
    command = <<-EOT
      set -eu
      region='${self.triggers.region}'
      echo "sockerless-lambda-sweep: region=$region"

      # List sockerless-managed Lambda functions. ListTags is per-ARN
      # so we walk every function once.
      for arn in $(aws lambda list-functions --region "$region" --query 'Functions[].FunctionArn' --output text); do
        [ -z "$arn" ] && continue
        managed=$(aws lambda list-tags --region "$region" --resource "$arn" --query 'Tags."sockerless-managed"' --output text 2>/dev/null || echo None)
        [ "$managed" != "true" ] && continue
        name=$(echo "$arn" | awk -F: '{print $NF}')
        # Clear VpcConfig so AWS Lambda releases the hyperplane ENI. If
        # the function has no VpcConfig this is a no-op. Wait briefly
        # for `LastUpdateStatus=Successful` so the subsequent delete
        # doesn't race.
        aws lambda update-function-configuration --region "$region" --function-name "$name" --vpc-config 'SubnetIds=[],SecurityGroupIds=[]' >/dev/null 2>&1 || true
        for j in 1 2 3 4 5; do
          status=$(aws lambda get-function --region "$region" --function-name "$name" --query 'Configuration.LastUpdateStatus' --output text 2>/dev/null || echo Failed)
          [ "$status" = "Successful" ] && break
          sleep 5
        done
        aws lambda delete-function --region "$region" --function-name "$name" >/dev/null 2>&1 || true
      done
    EOT
  }
}

# =============================================================================
# IAM — Lambda Execution Role
# =============================================================================

data "aws_iam_policy_document" "lambda_assume_role" {
  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["lambda.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "execution" {
  name               = "${local.name_prefix}-lambda-execution-role"
  assume_role_policy = data.aws_iam_policy_document.lambda_assume_role.json

  tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-lambda-execution-role"
  })
}

# AWSLambdaBasicExecutionRole — grants CloudWatch Logs permissions
resource "aws_iam_role_policy_attachment" "lambda_basic_execution" {
  role       = aws_iam_role.execution.name
  policy_arn = "arn:${data.aws_partition.current.partition}:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
}

# AWSLambdaVPCAccessExecutionRole — grants ec2:CreateNetworkInterface,
# DescribeNetworkInterfaces, DeleteNetworkInterface (and AssignPrivate
# IpAddresses + UnassignPrivateIpAddresses for ENI cleanup). Required
# whenever sockerless creates a Lambda with VpcConfig — the Lambda
# service uses these to attach an ENI in the function's subnets.
# Sockerless attaches VpcConfig whenever SOCKERLESS_LAMBDA_SUBNETS is
# set, which the live runbook does to share the ECS VPC.
resource "aws_iam_role_policy_attachment" "lambda_vpc_access" {
  role       = aws_iam_role.execution.name
  policy_arn = "arn:${data.aws_partition.current.partition}:iam::aws:policy/service-role/AWSLambdaVPCAccessExecutionRole"
}

# ECR pull permissions — allows Lambda to pull container images
data "aws_iam_policy_document" "ecr_pull" {
  statement {
    sid    = "ECRPull"
    effect = "Allow"
    actions = [
      "ecr:GetDownloadUrlForLayer",
      "ecr:BatchGetImage",
    ]
    resources = [aws_ecr_repository.main.arn]
  }

  statement {
    sid    = "ECRAuth"
    effect = "Allow"
    actions = [
      "ecr:GetAuthorizationToken",
    ]
    resources = ["*"]
  }
}

resource "aws_iam_role_policy" "ecr_pull" {
  name   = "${local.name_prefix}-lambda-ecr-pull"
  role   = aws_iam_role.execution.id
  policy = data.aws_iam_policy_document.ecr_pull.json
}

# =============================================================================
# CloudWatch Logs
# =============================================================================

resource "aws_cloudwatch_log_group" "main" {
  name              = "/sockerless/lambda/${local.name_prefix}"
  retention_in_days = var.log_retention_days

  tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-lambda-log-group"
  })
}

# =============================================================================
# ECR Repository
# =============================================================================

resource "aws_ecr_repository" "main" {
  name                 = "${local.name_prefix}-lambda"
  image_tag_mutability = "MUTABLE"

  # See ECS module for the rationale — destroy must succeed even with
  # pushed images so live-cloud teardowns don't need extra commands.
  force_delete = true

  image_scanning_configuration {
    scan_on_push = true
  }

  tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-lambda-ecr"
  })
}

resource "aws_ecr_lifecycle_policy" "main" {
  repository = aws_ecr_repository.main.name

  policy = jsonencode({
    rules = [
      {
        rulePriority = 1
        description  = "Expire untagged images after ${var.ecr_image_expiry_days} days"
        selection = {
          tagStatus   = "untagged"
          countType   = "sinceImagePushed"
          countUnit   = "days"
          countNumber = var.ecr_image_expiry_days
        }
        action = {
          type = "expire"
        }
      }
    ]
  })
}

# =============================================================================
# ECR Pull-through cache for Docker Hub
# =============================================================================
# AWS analogue of the GCP `docker-hub` Artifact Registry remote-proxy.
# Sockerless rewrites `docker.io/library/alpine:latest` to
# `<account>.dkr.ecr.<region>.amazonaws.com/docker-hub/library/alpine:latest`;
# the first pull populates the cache. Required for Lambda + ECS sub-task
# images that reference any Docker Hub base image (alpine, ubuntu, etc.).
#
# Pull-through cache rules are singleton per (account, region, prefix).
# When both lambda + ecs modules are deployed in the same account+region,
# set `manage_docker_hub_pull_through_cache = false` on one of them.
resource "aws_ecr_pull_through_cache_rule" "docker_hub" {
  count                 = var.manage_docker_hub_pull_through_cache ? 1 : 0
  ecr_repository_prefix = "docker-hub"
  upstream_registry_url = "registry-1.docker.io"
}
