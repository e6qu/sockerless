terraform {
  required_providers {
    google = {
      source = "hashicorp/google"
    }
  }
}

provider "google" {
  project = "test-project"
  region  = "us-central1"

  access_token              = "test-token"
  user_project_override     = false

  compute_custom_endpoint               = "${var.endpoint}/compute/v1/"
  dns_custom_endpoint                   = "${var.endpoint}/dns/v1/"
}

resource "google_compute_network" "main" {
  name                    = "tf-test-network"
  auto_create_subnetworks = false
}

resource "google_dns_managed_zone" "main" {
  name     = "tf-test-zone"
  dns_name = "tf-test.example.com."
}
