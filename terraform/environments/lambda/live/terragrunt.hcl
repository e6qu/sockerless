include "root" {
  path = find_in_parent_folders()
}

terraform {
  source = "../../../modules/lambda"
}

remote_state {
  backend = "s3"
  config = {
    bucket         = "sockerless-terraform-state"
    key            = "environments/lambda/live/terraform.tfstate"
    region         = "us-east-1"
    dynamodb_table = "sockerless-terraform-locks"
    encrypt        = true
  }
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite_terragrunt"
  }
}

inputs = {
  project_name          = "sockerless"
  environment           = "live"
  region                = "us-east-1"
  log_retention_days    = 7
  ecr_image_expiry_days = 7
  lambda_memory_size    = 512
  lambda_timeout        = 900
}
