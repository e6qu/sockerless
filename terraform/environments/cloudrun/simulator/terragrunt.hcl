include "root" {
  path = find_in_parent_folders()
}

terraform {
  source = "../../../modules/cloudrun"
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
  # The OAuth 2.0 access token the simulator's token endpoint issued for the
  # JWT-bearer grant; the harness mints it and hands it over as the provider's
  # own GOOGLE_OAUTH_ACCESS_TOKEN coordinate.
  access_token          = "${get_env("GOOGLE_OAUTH_ACCESS_TOKEN")}"
  user_project_override = false

  batching {
    send_after      = "0s"
    enable_batching = false
  }

  # Route every provider API call to the local Google Cloud simulator. Each
  # custom endpoint carries the version path the provider's client would
  # otherwise append itself; `google_service_account` speaks through the IAM
  # beta client, which honours only iam_beta_custom_endpoint.
  compute_custom_endpoint                   = "http://localhost:4567/compute/v1/"
  dns_custom_endpoint                       = "http://localhost:4567/dns/v1/"
  storage_custom_endpoint = "http://localhost:4567/storage/v1/"
  cloud_run_v2_custom_endpoint = "http://localhost:4567/v2/"
  artifact_registry_custom_endpoint = "http://localhost:4567/v1/"
  vpc_access_custom_endpoint               = "http://localhost:4567/v1/"
  service_usage_custom_endpoint = "http://localhost:4567/"
  iam_custom_endpoint = "http://localhost:4567/v1/"
  cloud_resource_manager_custom_endpoint    = "http://localhost:4567/"
  resource_manager_custom_endpoint = "http://localhost:4567/v1/"
  resource_manager_v3_custom_endpoint = "http://localhost:4567/v3/"
  logging_custom_endpoint = "http://localhost:4567/v2/"
  iam_beta_custom_endpoint = "http://localhost:4567/v1/"
}
EOF
}

inputs = {
  project_id                  = "sockerless-simulator"
  project_name                = "sockerless"
  environment                 = "simulator"
  region                      = "us-central1"
  vpc_connector_machine_type  = "e2-micro"
  vpc_connector_min_instances = 2
  vpc_connector_max_instances = 3
  gcs_location                = "US"
  gcs_lifecycle_days          = 1
}
