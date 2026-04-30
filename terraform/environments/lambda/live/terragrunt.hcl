include "root" {
  path = find_in_parent_folders()
}

terraform {
  source = "../../../modules/lambda"
}

# Match the ECS live env so Lambda can reuse the ECS VPC subnets +
# security group as documented in manual-tests/01-infrastructure.md.
# Same region (eu-west-1) and same state bucket (sockerless-tf-state).
generate "provider" {
  path      = "provider.tf"
  if_exists = "overwrite_terragrunt"
  contents  = <<EOF
provider "aws" {
  region = "eu-west-1"
}
EOF
}

remote_state {
  backend = "s3"
  config = {
    bucket  = "sockerless-tf-state"
    key     = "environments/lambda/live/terraform.tfstate"
    region  = "eu-west-1"
    encrypt = true
  }
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite_terragrunt"
  }
}

inputs = {
  project_name          = "sockerless"
  environment           = "live"
  region                = "eu-west-1"
  log_retention_days    = 7
  ecr_image_expiry_days = 7
  lambda_memory_size    = 512
  lambda_timeout        = 900
}
