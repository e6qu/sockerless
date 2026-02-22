# ---------------------------------------------------------------------------
# Module Outputs
# ---------------------------------------------------------------------------

output "project_id" {
  description = "GCP project ID"
  value       = var.project_id
}

output "region" {
  description = "GCP region"
  value       = var.region
}

# VPC

output "vpc_network_name" {
  description = "Name of the VPC network"
  value       = google_compute_network.main.name
}

output "vpc_network_id" {
  description = "ID of the VPC network"
  value       = google_compute_network.main.id
}

# VPC Connector

output "vpc_connector_name" {
  description = "Name of the Serverless VPC Access connector"
  value       = google_vpc_access_connector.main.name
}

output "vpc_connector_id" {
  description = "Fully qualified ID of the Serverless VPC Access connector"
  value       = google_vpc_access_connector.main.id
}

# Cloud DNS

output "dns_zone_name" {
  description = "Name of the Cloud DNS private managed zone"
  value       = google_dns_managed_zone.private.name
}

output "dns_zone_dns_name" {
  description = "DNS name of the Cloud DNS private managed zone"
  value       = google_dns_managed_zone.private.dns_name
}

# Cloud Storage

output "gcs_bucket_name" {
  description = "Name of the GCS bucket for volumes"
  value       = google_storage_bucket.volumes.name
}

output "gcs_bucket_url" {
  description = "URL of the GCS bucket for volumes"
  value       = google_storage_bucket.volumes.url
}

# Artifact Registry

output "artifact_registry_repository_name" {
  description = "Name of the Artifact Registry Docker repository"
  value       = google_artifact_registry_repository.main.repository_id
}

output "artifact_registry_repository_url" {
  description = "URL of the Artifact Registry Docker repository (region-docker.pkg.dev/project/repo)"
  value       = "${var.region}-docker.pkg.dev/${var.project_id}/${google_artifact_registry_repository.main.repository_id}"
}

# IAM

output "service_account_email" {
  description = "Email address of the Cloud Run runner service account"
  value       = google_service_account.runner.email
}

output "service_account_id" {
  description = "Fully qualified ID of the Cloud Run runner service account"
  value       = google_service_account.runner.id
}
