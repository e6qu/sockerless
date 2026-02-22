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

module "sockerless_ecs" {
  source = "../../../../terraform/modules/ecs"

  project_name       = var.project_name
  environment        = var.environment
  region             = var.region
  vpc_cidr           = var.vpc_cidr
  availability_zones = var.availability_zones
  log_retention_days = var.log_retention_days

  tags = {
    Example   = "true"
    ManagedBy = "terraform"
  }
}
