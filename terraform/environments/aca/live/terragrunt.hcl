include "root" {
  path = find_in_parent_folders()
}

terraform {
  source = "../../../modules/aca"
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
    key                  = "environments/aca/live/terraform.tfstate"
  }
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite_terragrunt"
  }
}

inputs = {
  project_name                     = "sockerless"
  environment                      = "live"
  location                         = "eastus"
  name_prefix                      = "sockerless"
  vnet_address_space               = ["10.0.0.0/16"]
  subnet_address_prefix            = "10.0.0.0/23"
  log_retention_days               = 30
  storage_account_replication_type = "LRS"
  acr_sku                          = "Basic"
  file_share_quota                 = 10
}
