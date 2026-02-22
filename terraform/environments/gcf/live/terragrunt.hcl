include "root" {
  path = find_in_parent_folders()
}

terraform {
  source = "../../../modules/gcf"
}

remote_state {
  backend = "gcs"
  config = {
    bucket   = "sockerless-terraform-state"
    prefix   = "environments/gcf/live"
    project  = "sockerless"
    location = "us"
  }
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite_terragrunt"
  }
}

inputs = {
  project_id   = "sockerless"
  project_name = "sockerless"
  environment  = "live"
  region       = "us-central1"
}
