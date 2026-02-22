# =============================================================================
# Cloud Functions (2nd gen) Terraform Module
# =============================================================================
#
# Provisions GCP infrastructure required by the Sockerless Cloud Functions
# backend. This includes:
#   - Enabling required APIs (Cloud Functions, Artifact Registry, Cloud Logging,
#     Cloud Build, Cloud Run)
#   - Artifact Registry Docker repository for container images
#   - IAM service account with least-privilege roles
#
# Cloud Functions 2nd gen is built on Cloud Run, so both APIs are required.
# Cloud Build is required because Cloud Functions uses it to build container
# images during deployment.
#
# Prerequisites:
#   - Google provider configured with appropriate credentials
#   - GCP project with billing enabled
#   - Terraform >= 1.5
#
# Usage:
#   module "gcf" {
#     source      = "../../modules/gcf"
#     project_id  = "my-gcp-project"
#     environment = "test"
#   }
# =============================================================================

locals {
  name_prefix = "${var.project_name}-${var.environment}"

  # GCP labels must be lowercase with only letters, numbers, hyphens,
  # and underscores.
  common_labels = merge(
    {
      project     = lower(var.project_name)
      environment = lower(var.environment)
      component   = "gcf"
      managed-by  = "terraform"
    },
    { for k, v in var.labels : lower(k) => lower(v) }
  )
}

# =============================================================================
# Enable Required APIs
# =============================================================================

resource "google_project_service" "cloudfunctions" {
  project = var.project_id
  service = "cloudfunctions.googleapis.com"

  disable_on_destroy = false
}

resource "google_project_service" "artifactregistry" {
  project = var.project_id
  service = "artifactregistry.googleapis.com"

  disable_on_destroy = false
}

resource "google_project_service" "logging" {
  project = var.project_id
  service = "logging.googleapis.com"

  disable_on_destroy = false
}

resource "google_project_service" "cloudbuild" {
  project = var.project_id
  service = "cloudbuild.googleapis.com"

  disable_on_destroy = false
}

# 2nd gen Cloud Functions run on Cloud Run
resource "google_project_service" "run" {
  project = var.project_id
  service = "run.googleapis.com"

  disable_on_destroy = false
}

# =============================================================================
# Artifact Registry — Docker Repository
# =============================================================================

resource "google_artifact_registry_repository" "main" {
  project       = var.project_id
  location      = var.region
  repository_id = "${local.name_prefix}-gcf"
  description   = "Docker repository for ${local.name_prefix} Cloud Functions container images"
  format        = "DOCKER"

  labels = local.common_labels

  depends_on = [google_project_service.artifactregistry]
}

# =============================================================================
# IAM — Service Account
# =============================================================================

resource "google_service_account" "main" {
  project      = var.project_id
  account_id   = "${local.name_prefix}-gcf-sa"
  display_name = "${local.name_prefix} Cloud Functions Service Account"
  description  = "Service account for Sockerless Cloud Functions backend (${var.environment})"
}

# =============================================================================
# IAM — Role Bindings
# =============================================================================

# Allow the service account to invoke Cloud Functions
resource "google_project_iam_member" "cloudfunctions_invoker" {
  project = var.project_id
  role    = "roles/cloudfunctions.invoker"
  member  = "serviceAccount:${google_service_account.main.email}"
}

# Allow the service account to write logs
resource "google_project_iam_member" "logging_writer" {
  project = var.project_id
  role    = "roles/logging.logWriter"
  member  = "serviceAccount:${google_service_account.main.email}"
}

# Allow the service account to read (pull) images from Artifact Registry
resource "google_project_iam_member" "artifactregistry_reader" {
  project = var.project_id
  role    = "roles/artifactregistry.reader"
  member  = "serviceAccount:${google_service_account.main.email}"
}
