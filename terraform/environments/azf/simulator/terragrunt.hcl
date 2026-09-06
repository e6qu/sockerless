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
  resource_provider_registrations = "none"
  use_cli                    = false
  use_msi                    = false

  # The simulator's bootstrap application registration and tenant — the
  # client credential every Azure client of the simulator presents.
  tenant_id       = "11111111-1111-1111-1111-111111111111"
  subscription_id = "00000000-0000-0000-0000-000000000001"
  client_id       = "test-client-id"
  client_secret   = "test-client-secret"
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
