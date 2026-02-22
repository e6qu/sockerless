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

# roles/artifactregistry.reader - Pull images from Artifact Registry
resource "google_artifact_registry_repository_iam_member" "runner_ar_reader" {
  project    = var.project_id
  location   = google_artifact_registry_repository.main.location
  repository = google_artifact_registry_repository.main.repository_id
  role       = "roles/artifactregistry.reader"
  member     = "serviceAccount:${google_service_account.runner.email}"
}
