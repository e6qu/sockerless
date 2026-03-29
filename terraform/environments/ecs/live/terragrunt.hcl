include "root" {
  path = find_in_parent_folders()
}

terraform {
  source = "../../../modules/ecs"
}

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
    bucket       = "sockerless-tf-state"
    key          = "environments/ecs/live/terraform.tfstate"
    region       = "eu-west-1"
    encrypt      = true
  }
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite_terragrunt"
  }
}

inputs = {
  project_name       = "sockerless"
  environment        = "live"
  region             = "eu-west-1"
  vpc_cidr           = "10.99.0.0/16"
  availability_zones = ["eu-west-1a", "eu-west-1b"]
  nat_gateway_count  = 1
  log_retention_days = 7
  ecr_image_expiry_days = 7
}
