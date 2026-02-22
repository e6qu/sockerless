terraform {
  required_version = ">= 1.5"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region = var.region
}

module "sockerless_lambda" {
  source = "../../../../terraform/modules/lambda"

  project_name       = var.project_name
  environment        = var.environment
  region             = var.region
  lambda_memory_size = var.lambda_memory_size
  lambda_timeout     = var.lambda_timeout
  log_retention_days = var.log_retention_days

  tags = {
    Example   = "true"
    ManagedBy = "terraform"
  }
}
