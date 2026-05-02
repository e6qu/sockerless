# Cloud Run Terraform Module
#
# Provisions all GCP infrastructure required by the Sockerless Cloud Run
# backend: API enablement, VPC networking with Serverless VPC Access
# connector, Cloud DNS private zone for service discovery, GCS bucket
# for volume mounts, Artifact Registry for container images, and an IAM
# service account with least-privilege roles.
#
# Prerequisites:
#   - An existing GCP project with billing enabled
#   - Terraform >= 1.5
#   - Google provider ~> 5.0
#
# Usage:
#   module "cloudrun" {
#     source      = "../../modules/cloudrun"
#     project_id  = "my-gcp-project"
#     environment = "test"
#   }

# ---------------------------------------------------------------------------
# Sockerless runtime sweep
# ---------------------------------------------------------------------------
# Sockerless creates Cloud Run services + jobs at runtime; they're not in
# this module's state. On destroy, sweep every sockerless-labeled
# resource so the IAM SA + VPC connector can be torn down without dangling
# references. Symmetric with the AWS ECS / Lambda module sweeps per the
# `[AWS teardown — terragrunt destroy must be self-sufficient]` rule.

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
      echo "sockerless-cloudrun-sweep: project=$project region=$region"

      # Cloud Run services: GCP labels use underscores
      # (sockerless_managed=true) per the per-cloud spelling rule.
      for svc in $(gcloud run services list --project="$project" --region="$region" --filter='metadata.labels.sockerless_managed=true' --format='value(metadata.name)' 2>/dev/null); do
        [ -z "$svc" ] && continue
        gcloud run services delete "$svc" --project="$project" --region="$region" --quiet >/dev/null 2>&1 || true
      done

      # Cloud Run jobs: same label rule.
      for job in $(gcloud run jobs list --project="$project" --region="$region" --filter='metadata.labels.sockerless_managed=true' --format='value(metadata.name)' 2>/dev/null); do
        [ -z "$job" ] && continue
        # Cancel running executions first (delete won't terminate them).
        for exec in $(gcloud run jobs executions list --job="$job" --project="$project" --region="$region" --filter='status.completionTime=NULL' --format='value(metadata.name)' 2>/dev/null); do
          [ -z "$exec" ] && continue
          gcloud run jobs executions cancel "$exec" --job="$job" --project="$project" --region="$region" --quiet >/dev/null 2>&1 || true
        done
        gcloud run jobs delete "$job" --project="$project" --region="$region" --quiet >/dev/null 2>&1 || true
      done
    EOT
  }
}

# ---------------------------------------------------------------------------
# API Enablement
# ---------------------------------------------------------------------------
# Enable required GCP APIs. disable_on_destroy = false prevents disabling
# shared APIs when this module is destroyed in a shared project.

resource "google_project_service" "run" {
  project            = var.project_id
  service            = "run.googleapis.com"
  disable_on_destroy = false
}

resource "google_project_service" "vpcaccess" {
  project            = var.project_id
  service            = "vpcaccess.googleapis.com"
  disable_on_destroy = false
}

resource "google_project_service" "dns" {
  project            = var.project_id
  service            = "dns.googleapis.com"
  disable_on_destroy = false
}

resource "google_project_service" "logging" {
  project            = var.project_id
  service            = "logging.googleapis.com"
  disable_on_destroy = false
}

resource "google_project_service" "artifactregistry" {
  project            = var.project_id
  service            = "artifactregistry.googleapis.com"
  disable_on_destroy = false
}

resource "google_project_service" "storage" {
  project            = var.project_id
  service            = "storage.googleapis.com"
  disable_on_destroy = false
}

# Cloud Build is the sockerless-sanctioned image builder for GCP — the
# cloudrun + gcf backends both call `gcpcommon.GCPBuildService` which
# drives Cloud Build via the GCS build-context bucket below. Required
# (no fallback) on every GCP project that runs sockerless.
resource "google_project_service" "cloudbuild" {
  project            = var.project_id
  service            = "cloudbuild.googleapis.com"
  disable_on_destroy = false
}

resource "google_project_service" "iam" {
  project            = var.project_id
  service            = "iam.googleapis.com"
  disable_on_destroy = false
}

# ---------------------------------------------------------------------------
# VPC Network
# ---------------------------------------------------------------------------

resource "google_compute_network" "main" {
  project                 = var.project_id
  name                    = "${local.name_prefix}-vpc"
  auto_create_subnetworks = false

  depends_on = [google_project_service.vpcaccess]
}

resource "google_compute_subnetwork" "connector" {
  project       = var.project_id
  name          = "${local.name_prefix}-connector-subnet"
  region        = var.region
  network       = google_compute_network.main.id
  ip_cidr_range = "10.8.0.0/28"
}

# ---------------------------------------------------------------------------
# Serverless VPC Access Connector
# ---------------------------------------------------------------------------
# Bridges Cloud Run Jobs to VPC resources (Cloud DNS, other services).

resource "google_vpc_access_connector" "main" {
  project       = var.project_id
  name          = "${local.name_prefix}-conn"
  region        = var.region
  machine_type  = var.vpc_connector_machine_type
  min_instances = var.vpc_connector_min_instances
  max_instances = var.vpc_connector_max_instances

  subnet {
    name = google_compute_subnetwork.connector.name
  }

  depends_on = [google_project_service.vpcaccess]
}

# ---------------------------------------------------------------------------
# Cloud DNS Private Managed Zone
# ---------------------------------------------------------------------------
# Private DNS zone for service discovery. Only resolvable from within the
# associated VPC, so Cloud Run Jobs must use the VPC connector.

resource "google_dns_managed_zone" "private" {
  project     = var.project_id
  name        = "${local.name_prefix}-dns"
  dns_name    = "${var.dns_suffix}."
  description = "Private DNS zone for ${var.project_name} ${var.environment} service discovery"
  visibility  = "private"
  labels      = local.common_labels

  private_visibility_config {
    networks {
      network_url = google_compute_network.main.id
    }
  }

  depends_on = [google_project_service.dns]
}

# ---------------------------------------------------------------------------
# Cloud Storage (GCS) Bucket
# ---------------------------------------------------------------------------
# Used for volume mounts via Cloud Storage FUSE in Cloud Run Jobs.

resource "google_storage_bucket" "volumes" {
  project                     = var.project_id
  name                        = "${local.name_prefix}-volumes"
  location                    = var.gcs_location
  uniform_bucket_level_access = true
  labels                      = local.common_labels
  force_destroy               = true

  lifecycle_rule {
    condition {
      age = var.gcs_lifecycle_days
    }
    action {
      type = "Delete"
    }
  }

  depends_on = [google_project_service.storage]
}

# ---------------------------------------------------------------------------
# Cloud Build context bucket
# ---------------------------------------------------------------------------
# Sockerless backend's runtime image-build path uploads the build
# context as a tarball to this bucket; Cloud Build downloads it,
# builds the image (per-arch + manifest list), pushes to AR. This
# bucket is the GCP analogue of the AWS lambda module's
# `aws_s3_bucket.build_context`. Lifecycle policy mirrors AWS:
# build contexts are single-use, expire after 1 day.
#
# The dispatcher passes this bucket name on the runner Cloud Run Job
# as `SOCKERLESS_GCP_BUILD_BUCKET` (required env var; bootstrap.sh
# fails loudly if missing).

resource "google_storage_bucket" "build_context" {
  project                     = var.project_id
  name                        = "${local.name_prefix}-build-context"
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

# ---------------------------------------------------------------------------
# Artifact Registry
# ---------------------------------------------------------------------------
# Docker repository for the sockerless-agent image and loaded container images.

resource "google_artifact_registry_repository" "main" {
  project       = var.project_id
  location      = var.region
  repository_id = "${local.name_prefix}-repo"
  format        = "DOCKER"
  description   = "Docker repository for ${var.project_name} ${var.environment}"
  labels        = local.common_labels

  cleanup_policies {
    id     = "delete-untagged"
    action = "DELETE"

    condition {
      tag_state  = "UNTAGGED"
      older_than = "604800s" # 7 days
    }
  }

  depends_on = [google_project_service.artifactregistry]
}

# Remote-Docker-Hub-proxy repository named exactly `docker-hub` —
# `gcpcommon.ResolveGCPImageURI` rewrites Docker Hub refs to
# `{region}-docker.pkg.dev/{project}/docker-hub/{repo}:{tag}`. Without
# this repo every `docker run alpine` against real GCP fails with
# `Image not found`.
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

# ---------------------------------------------------------------------------
# IAM Service Account
# ---------------------------------------------------------------------------
# Service account for Cloud Run job executions with least-privilege roles.

resource "google_service_account" "runner" {
  project      = var.project_id
  account_id   = "${local.name_prefix}-runner"
  display_name = "${var.project_name} ${var.environment} Cloud Run Runner"
}

# roles/run.invoker - Start job executions
resource "google_project_iam_member" "runner_run_invoker" {
  project = var.project_id
  role    = "roles/run.invoker"
  member  = "serviceAccount:${google_service_account.runner.email}"
}

# roles/logging.logWriter - Write logs to Cloud Logging
resource "google_project_iam_member" "runner_log_writer" {
  project = var.project_id
  role    = "roles/logging.logWriter"
  member  = "serviceAccount:${google_service_account.runner.email}"
}

# roles/storage.objectAdmin - Read/write GCS bucket for volumes
resource "google_storage_bucket_iam_member" "runner_storage_admin" {
  bucket = google_storage_bucket.volumes.name
  role   = "roles/storage.objectAdmin"
  member = "serviceAccount:${google_service_account.runner.email}"
}

# roles/dns.admin - Manage DNS records in the private zone
resource "google_project_iam_member" "runner_dns_admin" {
  project = var.project_id
  role    = "roles/dns.admin"
  member  = "serviceAccount:${google_service_account.runner.email}"
}

# roles/artifactregistry.writer - Push runtime-built images to AR.
# Required because the in-image sockerless backend runs `docker build`
# at runtime via Cloud Build and pushes to AR (per-arch + manifest list).
resource "google_artifact_registry_repository_iam_member" "runner_ar_writer" {
  project    = var.project_id
  location   = google_artifact_registry_repository.main.location
  repository = google_artifact_registry_repository.main.repository_id
  role       = "roles/artifactregistry.writer"
  member     = "serviceAccount:${google_service_account.runner.email}"
}

# roles/cloudbuild.builds.editor - Submit Cloud Build jobs from the
# in-image sockerless backend (`gcpcommon.GCPBuildService.Build`).
resource "google_project_iam_member" "runner_cloudbuild_editor" {
  project = var.project_id
  role    = "roles/cloudbuild.builds.editor"
  member  = "serviceAccount:${google_service_account.runner.email}"
}

# roles/run.admin - Create / start / delete Cloud Run Jobs at runtime
# (the in-image sockerless cloudrun backend dispatches sub-tasks as
# Cloud Run Jobs).
resource "google_project_iam_member" "runner_run_admin" {
  project = var.project_id
  role    = "roles/run.admin"
  member  = "serviceAccount:${google_service_account.runner.email}"
}

# roles/iam.serviceAccountUser - Allow the runner SA to act as itself
# when creating Cloud Run Jobs that run under the same SA. Required
# by the Cloud Run + Cloud Build APIs whenever a service is created
# that runs as a particular SA.
resource "google_service_account_iam_member" "runner_act_as_self" {
  service_account_id = google_service_account.runner.id
  role               = "roles/iam.serviceAccountUser"
  member             = "serviceAccount:${google_service_account.runner.email}"
}

# roles/storage.admin (scoped to build_context bucket) - Upload build
# contexts; Cloud Build downloads them.
resource "google_storage_bucket_iam_member" "runner_build_bucket_admin" {
  bucket = google_storage_bucket.build_context.name
  role   = "roles/storage.admin"
  member = "serviceAccount:${google_service_account.runner.email}"
}

# roles/logging.viewer - Read sub-task logs back to surface them via
# `docker logs` on the in-image backend.
resource "google_project_iam_member" "runner_logging_viewer" {
  project = var.project_id
  role    = "roles/logging.viewer"
  member  = "serviceAccount:${google_service_account.runner.email}"
}
