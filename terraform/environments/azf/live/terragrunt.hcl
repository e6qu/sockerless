include "root" {
  path = find_in_parent_folders()
}

terraform {
  source = "../../../modules/azf"
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
