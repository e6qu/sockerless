terraform {
  required_version = ">= 1.5"
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
  }
}

provider "google" {
  project = var.project_id
  region  = var.region
}

module "sockerless_gcf" {
  source = "../../../../terraform/modules/gcf"

  project_id   = var.project_id
  region       = var.region
  project_name = var.project_name
  environment  = var.environment

  labels = {
    example    = "true"
    managed-by = "terraform"
  }
}
