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
