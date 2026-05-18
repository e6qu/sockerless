terraform {
  required_providers {
    google = {
      source = "hashicorp/google"
    }
    random = {
      source = "hashicorp/random"
    }
  }
}

provider "google" {
  project = "test-project"
  region  = "us-central1"

  access_token          = "test-token"
  user_project_override = false

  compute_custom_endpoint           = "${var.endpoint}/compute/v1/"
  dns_custom_endpoint               = "${var.endpoint}/dns/v1/"
  artifact_registry_custom_endpoint = "${var.endpoint}/v1/"
  cloud_run_v2_custom_endpoint      = "${var.endpoint}/v2/"
  storage_custom_endpoint           = "${var.endpoint}/storage/v1/"
  secret_manager_custom_endpoint    = "${var.endpoint}/v1/"
  # iam_beta_custom_endpoint routes the `google_service_account` resource's
  # iambeta.NewClient → iam.googleapis.com surface; without it the resource
  # hits real iam.googleapis.com regardless of `iam_custom_endpoint`.
  iam_beta_custom_endpoint = "${var.endpoint}/v1/"
}

# ---------- Compute (network + disks) ----------

resource "google_compute_network" "main" {
  name                    = "tf-test-network"
  auto_create_subnetworks = false
}

# Compute disk CRUD. Exercises the same wire shape gcloud uses
# (zoneOp with operationType, full zone URL, kind=compute#operation)
# from a third consumer.
resource "google_compute_disk" "tf_disk" {
  name = "tf-test-disk"
  zone = "us-central1-a"
  size = 10
  type = "pd-balanced"
}

# Subnetwork inside the test network. Real runner pods get attached to
# subnets; the sim's POST /compute/v1/projects/{}/regions/{}/subnetworks
# is the wire surface terraform-provider-google calls.
resource "google_compute_subnetwork" "tf_subnet" {
  name          = "tf-test-subnet"
  region        = "us-central1"
  network       = google_compute_network.main.id
  ip_cidr_range = "10.42.0.0/16"
}

# Global firewall rule attached to the network. Runner backends create
# matching allow-rules for SSH / health-check ranges; this exercises the
# /compute/v1/projects/{}/global/firewalls surface.
resource "google_compute_firewall" "tf_fw" {
  name    = "tf-test-fw-allow-ssh"
  network = google_compute_network.main.name

  allow {
    protocol = "tcp"
    ports    = ["22"]
  }

  source_ranges = ["0.0.0.0/0"]
}

# ---------- DNS (public + private zone) ----------

resource "google_dns_managed_zone" "main" {
  name     = "tf-test-zone"
  dns_name = "tf-test.example.com."
}

# Private managed zone auto-backs itself with a real Docker network
# inside the simulator (see simulators/gcp/dns.go handleCreateZone).
# Terraform round-trip covers zone Create + Get + Delete — enough to
# prove the `Visibility=private` path triggers the simulator's network
# lifecycle.
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
  description = "cross-job DNS coverage"
}

# ---------- Artifact Registry (Docker repo + cleanup policy) ----------

# Docker-format repository — backs both Cloud Run image pulls and the
# OCI distribution proxy at `/v2/{repo}/manifests/...`. Real production
# stacks reach for this in their pre-deploy step before `google_cloud_run_v2_service`.
resource "google_artifact_registry_repository" "tf_ar_docker" {
  location      = "us-central1"
  repository_id = "tf-ar-docker"
  description   = "tf-test Docker repository"
  format        = "DOCKER"
}

# Remote Docker Hub repository — same Artifact Registry shape the
# production Cloud Run/GCF modules use for Docker Hub image resolution.
resource "google_artifact_registry_repository" "tf_ar_docker_hub" {
  location      = "us-central1"
  repository_id = "docker-hub"
  description   = "tf-test Docker Hub remote repository"
  format        = "DOCKER"
  mode          = "REMOTE_REPOSITORY"

  remote_repository_config {
    description = "Proxies docker.io / Docker Hub"
    docker_repository {
      public_repository = "DOCKER_HUB"
    }
  }
}

# ---------- Cloud Run v2 (Service + Job) ----------

resource "google_cloud_run_v2_service" "tf_crv2_svc" {
  name                = "tf-crv2-svc"
  location            = "us-central1"
  deletion_protection = false

  template {
    containers {
      image = "us-central1-docker.pkg.dev/test-project/tf-ar-docker/test:latest"

      ports {
        container_port = 8080
      }

      env {
        name  = "TF_TEST"
        value = "true"
      }
    }

    scaling {
      min_instance_count = 0
      max_instance_count = 3
    }
  }

  depends_on = [google_artifact_registry_repository.tf_ar_docker]
}

resource "google_cloud_run_v2_job" "tf_crv2_job" {
  name                = "tf-crv2-job"
  location            = "us-central1"
  deletion_protection = false

  template {
    template {
      containers {
        image = "us-central1-docker.pkg.dev/test-project/tf-ar-docker/job:latest"
      }
    }
  }

  depends_on = [google_artifact_registry_repository.tf_ar_docker]
}

# ---------- Cloud Storage ----------

resource "google_storage_bucket" "tf_bucket" {
  name          = "tf-test-bucket-${random_id.bucket_suffix.hex}"
  location      = "us-central1"
  force_destroy = true

  uniform_bucket_level_access = true
}

resource "random_id" "bucket_suffix" {
  byte_length = 4
}

# GCS object inside the bucket — runner workflows stage build artifacts
# / function source archives here. Exercises POST /upload/storage/v1/b/{bucket}/o.
resource "google_storage_bucket_object" "tf_artifact" {
  name    = "tf-test-artifact.txt"
  bucket  = google_storage_bucket.tf_bucket.name
  content = "tf-test-payload"
}

# ---------- Secret Manager ----------

resource "google_secret_manager_secret" "tf_secret" {
  secret_id = "tf-test-secret"

  replication {
    auto {}
  }
}

resource "google_secret_manager_secret_version" "tf_secret_v1" {
  secret      = google_secret_manager_secret.tf_secret.id
  secret_data = "tf-test-secret-payload"
}

# ---------- IAM (service account) ----------

# Service accounts are the runner identity primitive. The sim exposes
# POST /v1/projects/{}/serviceAccounts; terraform-provider-google routes
# google_service_account through iambeta.NewClient which is configured
# via iam_beta_custom_endpoint (above).
resource "google_service_account" "tf_sa" {
  account_id   = "tf-test-runner-sa"
  display_name = "tf-test runner service account"
}

# ---------- Outputs (cross-resource invariants) ----------

output "compute_disk_self_link" {
  value = google_compute_disk.tf_disk.self_link
}

output "dns_zone_name_servers" {
  value = google_dns_managed_zone.main.name_servers
}

output "ar_repo_id" {
  value = google_artifact_registry_repository.tf_ar_docker.id
}

output "ar_remote_repo_id" {
  value = google_artifact_registry_repository.tf_ar_docker_hub.id
}

output "cloud_run_v2_service_uri" {
  value = google_cloud_run_v2_service.tf_crv2_svc.uri
}

output "cloud_run_v2_job_id" {
  value = google_cloud_run_v2_job.tf_crv2_job.id
}

output "storage_bucket_url" {
  value = google_storage_bucket.tf_bucket.url
}

output "secret_version_id" {
  value = google_secret_manager_secret_version.tf_secret_v1.id
}

output "subnet_id" {
  value = google_compute_subnetwork.tf_subnet.id
}

output "firewall_id" {
  value = google_compute_firewall.tf_fw.id
}

output "gcs_object_self_link" {
  value = google_storage_bucket_object.tf_artifact.self_link
}

output "service_account_email" {
  value = google_service_account.tf_sa.email
}

output "service_account_name" {
  value = google_service_account.tf_sa.name
}
