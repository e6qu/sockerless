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

module "sockerless_azf" {
  source = "../../../../terraform/modules/azf"

  project_name = var.project_name
  environment  = var.environment
  location     = var.location

  tags = {
    Example   = "true"
    ManagedBy = "terraform"
  }
}
