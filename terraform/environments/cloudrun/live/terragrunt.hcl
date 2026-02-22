include "root" {
  path = find_in_parent_folders()
}

terraform {
  source = "../../../modules/cloudrun"
}

remote_state {
  backend = "gcs"
  config = {
    bucket   = "sockerless-terraform-state"
    prefix   = "environments/cloudrun/live"
    project  = "sockerless"
    location = "us"
  }
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite_terragrunt"
  }
}

inputs = {
  project_id                  = "sockerless"
  project_name                = "sockerless"
  environment                 = "live"
  region                      = "us-central1"
  vpc_connector_machine_type  = "e2-micro"
  vpc_connector_min_instances = 2
  vpc_connector_max_instances = 3
  gcs_location                = "US"
  gcs_lifecycle_days          = 30
}
