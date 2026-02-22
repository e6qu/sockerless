include "root" {
  path = find_in_parent_folders()
}

terraform {
  source = "../../../modules/ecs"
}

remote_state {
  backend = "s3"
  config = {
    bucket         = "sockerless-terraform-state"
    key            = "environments/ecs/live/terraform.tfstate"
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
  project_name       = "sockerless"
  environment        = "live"
  region             = "us-east-1"
  vpc_cidr           = "10.99.0.0/16"
  availability_zones = ["us-east-1a", "us-east-1b"]
  nat_gateway_count  = 1
  log_retention_days = 7
  ecr_image_expiry_days = 7
}
