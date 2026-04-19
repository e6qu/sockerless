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

# BUG-701 on GCP: a private managed zone auto-backs itself with a
# real Docker network inside the simulator (see simulators/gcp/dns.go
# handleCreateZone). The terraform round-trip covers zone Create +
# Get + Delete — enough to prove the `Visibility=private` path
# triggers the simulator's network lifecycle.
#
# Record-set terraform coverage is intentionally omitted: the google
# provider's ResourceRecordSets client reconstructs the endpoint URL
# from the host/port only (ignoring the /dns/v1/ path from
# dns_custom_endpoint), causing a plugin panic against the simulator.
# The SDK + CLI tests cover the record-set-connects-container flow.
resource "google_dns_managed_zone" "private_xjob" {
  name        = "tf-xjob-zone"
  dns_name    = "tf-xjob.local."
  visibility  = "private"
  description = "Phase 86 cross-job DNS (BUG-701 on GCP)"
}
