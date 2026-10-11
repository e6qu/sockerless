include "root" {
  path = find_in_parent_folders()
}

terraform {
  source = "../../../modules/azf"
}

# The azurerm provider requires a `features` block; the subscription and the
# credential come from the operator's environment (ARM_SUBSCRIPTION_ID, and
# the Azure CLI login or ARM_CLIENT_ID / ARM_TENANT_ID with a secret or a
# federated token), as they do for every azurerm configuration.
generate "provider" {
  path      = "provider.tf"
  if_exists = "overwrite_terragrunt"
  contents  = <<EOF
provider "azurerm" {
  features {}
}
EOF
}

remote_state {
  backend = "azurerm"
  config = {
    resource_group_name  = "sockerless-terraform-state"
    storage_account_name = "sockerlesstfstate"
    container_name       = "tfstate"
    key                  = "environments/azf/live/terraform.tfstate"
  }
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite_terragrunt"
  }
}

inputs = {
  project_name             = "sockerless"
  environment              = "live"
  location                 = "eastus"
  name_prefix              = "sockerless"
  storage_replication_type = "LRS"
  acr_sku                  = "Basic"
  app_service_plan_sku     = "Y1"
  log_retention_days       = 30
}
