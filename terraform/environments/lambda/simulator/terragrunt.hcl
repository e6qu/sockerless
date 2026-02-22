include "root" {
  path = find_in_parent_folders()
}

terraform {
  source = "../../../modules/lambda"
}

# Simulator environment uses local state (no real cloud)
remote_state {
  backend = "local"
  config = {
    path = "${get_terragrunt_dir()}/terraform.tfstate"
  }
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite_terragrunt"
  }
}

# Override the provider to point at the AWS simulator (LocalStack)
generate "provider_override" {
  path      = "provider_override.tf"
  if_exists = "overwrite_terragrunt"
  contents  = <<EOF
provider "aws" {
  region                      = "us-east-1"
  access_key                  = "test"
  secret_key                  = "test"
  skip_credentials_validation = true
  skip_metadata_api_check     = true
  skip_requesting_account_id  = true

  endpoints {
    lambda         = "http://localhost:4566"
    cloudwatchlogs = "http://localhost:4566"
    ecr            = "http://localhost:4566"
    s3             = "http://localhost:4566"
    iam            = "http://localhost:4566"
    sts            = "http://localhost:4566"
    apigatewayv2   = "http://localhost:4566"
    sqs            = "http://localhost:4566"
  }
}
EOF
}

inputs = {
  project_name          = "sockerless"
  environment           = "simulator"
  region                = "us-east-1"
  log_retention_days    = 1
  ecr_image_expiry_days = 1
  lambda_memory_size    = 512
  lambda_timeout        = 900
}
