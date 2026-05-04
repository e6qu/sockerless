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

# ---------------------------------------------------------------------------
# Sockerless runtime sweep
# ---------------------------------------------------------------------------
# Sockerless creates Cloud Functions (gen2) at runtime; they're not in
# this module's state. On destroy, sweep every sockerless-labeled
# function so the IAM service account + Artifact Registry repo can be
# torn down cleanly. Symmetric with the AWS / Cloud Run / Azure module
# sweeps per the project teardown rule.
resource "null_resource" "sockerless_runtime_sweep" {
  triggers = {
    project = var.project_id
    region  = var.region
  }

  provisioner "local-exec" {
    when    = destroy
    command = <<-EOT
      set -eu
      project='${self.triggers.project}'
      region='${self.triggers.region}'
      echo "sockerless-gcf-sweep: project=$project region=$region"

      # GCP labels use underscores per per-cloud spelling rule
      # (sockerless_managed=true).
      for fn in $(gcloud functions list --project="$project" --regions="$region" --filter='labels.sockerless_managed=true' --format='value(name)' 2>/dev/null); do
        [ -z "$fn" ] && continue
        gcloud functions delete "$fn" --project="$project" --region="$region" --quiet --gen2 >/dev/null 2>&1 || \
          gcloud functions delete "$fn" --project="$project" --region="$region" --quiet >/dev/null 2>&1 || true
      done
    EOT
  }
}

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

# Cloud Storage for the Cloud Build context bucket below.
resource "google_project_service" "storage" {
  project = var.project_id
  service = "storage.googleapis.com"

  disable_on_destroy = false
}

resource "google_project_service" "iam" {
  project            = var.project_id
  service            = "iam.googleapis.com"
  disable_on_destroy = false
}

# =============================================================================
# Cloud Build context bucket
# =============================================================================
# Sockerless backend's runtime image-build path uploads the build
# context as a tarball to this bucket; Cloud Build downloads it,
# builds the image, pushes to AR. Same shape as the cloudrun
# module's build_context bucket. The dispatcher passes this bucket
# name on the runner Cloud Run Job as `SOCKERLESS_GCP_BUILD_BUCKET`
# (required env var; bootstrap.sh fails loudly if missing).

resource "google_storage_bucket" "build_context" {
  project                     = var.project_id
  name                        = "${local.name_prefix}-gcf-build-context"
  location                    = var.gcs_location
  uniform_bucket_level_access = true
  labels                      = local.common_labels
  force_destroy               = true

  lifecycle_rule {
    condition {
      age = 1
    }
    action {
      type = "Delete"
    }
  }

  depends_on = [google_project_service.storage]
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

# Remote-Docker-Hub-proxy repository named exactly `docker-hub` —
# `gcpcommon.ResolveGCPImageURI` rewrites Docker Hub refs to
# `{region}-docker.pkg.dev/{project}/docker-hub/{repo}:{tag}`. Without
# this repo every `docker run alpine` through the gcf backend fails
# with `Image not found`. Shared with the cloudrun module when both
# are deployed in the same project.
resource "google_artifact_registry_repository" "docker_hub" {
  project       = var.project_id
  location      = var.region
  repository_id = "docker-hub"
  format        = "DOCKER"
  mode          = "REMOTE_REPOSITORY"
  description   = "Docker Hub proxy for sockerless image-resolve"

  remote_repository_config {
    description = "Proxies docker.io / Docker Hub"
    docker_repository {
      public_repository = "DOCKER_HUB"
    }
  }

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

# Allow the service account to push runtime-built images to AR
# (sockerless backend's gcpcommon.GCPBuildService writes here).
resource "google_project_iam_member" "artifactregistry_writer" {
  project = var.project_id
  role    = "roles/artifactregistry.writer"
  member  = "serviceAccount:${google_service_account.main.email}"
}

# Allow the service account to submit Cloud Build jobs (the
# sockerless-sanctioned image builder for GCP).
resource "google_project_iam_member" "cloudbuild_editor" {
  project = var.project_id
  role    = "roles/cloudbuild.builds.editor"
  member  = "serviceAccount:${google_service_account.main.email}"
}

# Allow the service account to deploy / update Cloud Functions (gen2)
# at runtime — the in-image sockerless gcf backend creates one
# function per sub-task.
resource "google_project_iam_member" "cloudfunctions_developer" {
  project = var.project_id
  role    = "roles/cloudfunctions.developer"
  member  = "serviceAccount:${google_service_account.main.email}"
}

# 2nd-gen Cloud Functions run on Cloud Run, so the SA also needs
# Cloud Run admin to create the underlying Cloud Run service.
resource "google_project_iam_member" "run_admin" {
  project = var.project_id
  role    = "roles/run.admin"
  member  = "serviceAccount:${google_service_account.main.email}"
}

# Allow the SA to act as itself when creating Cloud Functions /
# Cloud Run services that run under the same SA.
resource "google_service_account_iam_member" "act_as_self" {
  service_account_id = google_service_account.main.id
  role               = "roles/iam.serviceAccountUser"
  member             = "serviceAccount:${google_service_account.main.email}"
}

# Allow uploads to the Cloud Build context bucket.
resource "google_storage_bucket_iam_member" "build_bucket_admin" {
  bucket = google_storage_bucket.build_context.name
  role   = "roles/storage.admin"
  member = "serviceAccount:${google_service_account.main.email}"
}

# Allow reading sub-task logs back via Cloud Logging.
resource "google_project_iam_member" "logging_viewer" {
  project = var.project_id
  role    = "roles/logging.viewer"
  member  = "serviceAccount:${google_service_account.main.email}"
}
