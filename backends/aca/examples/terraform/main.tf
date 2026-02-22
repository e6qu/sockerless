terraform {
  required_version = ">= 1.5"
  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 3.0"
    }
  }
}

provider "azurerm" {
  features {}
}

module "sockerless_aca" {
  source = "../../../../terraform/modules/aca"

  project_name = var.project_name
  environment  = var.environment
  location     = var.location

  tags = {
    Example   = "true"
    ManagedBy = "terraform"
  }
}
