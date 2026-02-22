include "root" {
  path = find_in_parent_folders()
}

terraform {
  source = "../../../modules/azf"
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

# Override the provider to point at the Azure simulator
generate "provider_override" {
  path      = "provider_override.tf"
  if_exists = "overwrite_terragrunt"
  contents  = <<EOF
provider "azurerm" {
  features {}

  # Point at the local Azure simulator.
  # The simulator must run with TLS (SIM_TLS_CERT/SIM_TLS_KEY).
  # Set ARM_METADATA_HOST=localhost:4568 and SSL_CERT_FILE to the CA cert.
  skip_provider_registration = true
  use_cli                    = false
  use_msi                    = false

  tenant_id       = "00000000-0000-0000-0000-000000000000"
  subscription_id = "00000000-0000-0000-0000-000000000000"
  client_id       = "00000000-0000-0000-0000-000000000000"
  client_secret   = "test"
}
EOF
}

inputs = {
  project_name             = "sockerless"
  environment              = "simulator"
  location                 = "eastus"
  name_prefix              = "sockerless"
  storage_replication_type = "LRS"
  acr_sku                  = "Basic"
  app_service_plan_sku     = "Y1"
  log_retention_days       = 30
}
