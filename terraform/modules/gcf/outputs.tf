# =============================================================================
# Project
# =============================================================================

output "project_id" {
  description = "GCP project ID where resources are deployed"
  value       = var.project_id
}

output "region" {
  description = "GCP region where resources are deployed"
  value       = var.region
}

# =============================================================================
# Artifact Registry
# =============================================================================

output "artifact_registry_repository_url" {
  description = "URL of the Artifact Registry Docker repository for Cloud Functions container images"
  value       = "${var.region}-docker.pkg.dev/${var.project_id}/${google_artifact_registry_repository.main.repository_id}"
}

# =============================================================================
# IAM
# =============================================================================

output "service_account_email" {
  description = "Email of the service account for Cloud Functions"
  value       = google_service_account.main.email
}
