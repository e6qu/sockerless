include "root" {
  path = find_in_parent_folders()
}

terraform {
  source = "../../../modules/gcf"
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

# Override the provider to point at the GCP simulator
generate "provider_override" {
  path      = "provider_override.tf"
  if_exists = "overwrite_terragrunt"
  contents  = <<EOF
provider "google" {
  project     = "sockerless-simulator"
  region      = "us-central1"
  access_token          = "test-token"
  user_project_override = false

  batching {
    send_after      = "0s"
    enable_batching = false
  }

  # Route all provider API calls to the local GCP simulator.
  # All custom endpoints must match .*/[^/]+/$
  cloud_functions_custom_endpoint            = "http://localhost:4567/v2/"
  cloud_run_v2_custom_endpoint               = "http://localhost:4567/v2/"
  artifact_registry_custom_endpoint          = "http://localhost:4567/v1/"
  service_usage_custom_endpoint              = "http://localhost:4567/"
  iam_custom_endpoint                        = "http://localhost:4567/"
  cloud_resource_manager_custom_endpoint     = "http://localhost:4567/"
  resource_manager_custom_endpoint           = "http://localhost:4567/"
  resource_manager_v3_custom_endpoint        = "http://localhost:4567/"
  logging_custom_endpoint                    = "http://localhost:4567/"
}
EOF
}

inputs = {
  project_id   = "sockerless-simulator"
  project_name = "sockerless"
  environment  = "simulator"
  region       = "us-central1"
}
