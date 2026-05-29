terraform {
  required_providers {
    archive = {
      source = "hashicorp/archive"
    }
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
  big_query_custom_endpoint         = "${var.endpoint}/bigquery/v2/"
  cloud_build_custom_endpoint       = "${replace(var.endpoint, "127.0.0.1", "cloudbuild.localhost")}/v1/"
  cloudfunctions2_custom_endpoint   = "${var.endpoint}/v2/"
  dns_custom_endpoint               = "${var.endpoint}/dns/v1/"
  artifact_registry_custom_endpoint = "${var.endpoint}/v1/"
  cloud_run_v2_custom_endpoint      = "${var.endpoint}/v2/"
  eventarc_custom_endpoint          = "${replace(var.endpoint, "127.0.0.1", "eventarc.localhost")}/v1/"
  firestore_custom_endpoint         = "${var.endpoint}/v1/"
  logging_custom_endpoint           = "${var.endpoint}/v2/"
  pubsub_custom_endpoint            = "${var.endpoint}/v1/"
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

resource "google_compute_instance" "tf_vm" {
  name         = "tf-test-vm"
  machine_type = "e2-micro"
  zone         = "us-central1-a"

  boot_disk {
    initialize_params {
      image = "projects/debian-cloud/global/images/debian-12"
      size  = 10
    }
  }

  network_interface {
    subnetwork = google_compute_subnetwork.tf_subnet.id
  }

  labels = {
    env = "terraform"
  }
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

# ---------- Cloud Functions v2 ----------

data "archive_file" "tf_function_source" {
  type        = "zip"
  output_path = "${path.module}/.terraform/function-source.zip"

  source {
    filename = "index.js"
    content  = <<-EOT
      exports.helloHttp = (req, res) => {
        res.status(200).send("ok");
      };
    EOT
  }
}

resource "google_storage_bucket_object" "tf_function_source" {
  name   = "function-source.zip"
  bucket = google_storage_bucket.tf_bucket.name
  source = data.archive_file.tf_function_source.output_path
}

resource "google_cloudfunctions2_function" "tf_gcfv2_function" {
  name        = "tf-gcfv2-function"
  location    = "us-central1"
  description = "Terraform simulator coverage for Cloud Functions v2"

  build_config {
    runtime     = "nodejs20"
    entry_point = "helloHttp"
    source {
      storage_source {
        bucket = google_storage_bucket.tf_bucket.name
        object = google_storage_bucket_object.tf_function_source.name
      }
    }
  }

  service_config {
    max_instance_count = 1
    available_memory   = "256M"
    timeout_seconds    = 60
  }

  depends_on = [google_storage_bucket_object.tf_function_source]
}

# ---------- Eventarc ----------

resource "google_eventarc_trigger" "tf_eventarc_trigger" {
  name     = "tf-eventarc-trigger"
  location = "us-central1"

  matching_criteria {
    attribute = "type"
    value     = "google.cloud.pubsub.topic.v1.messagePublished"
  }

  destination {
    cloud_run_service {
      service = google_cloud_run_v2_service.tf_crv2_svc.name
      region  = "us-central1"
    }
  }

  labels = {
    env = "test"
  }

  depends_on = [google_cloud_run_v2_service.tf_crv2_svc]
}

resource "google_eventarc_channel" "tf_eventarc_channel" {
  name     = "tf-eventarc-channel"
  location = "us-central1"

  labels = {
    env = "test"
  }
}

# ---------- Pub/Sub ----------

resource "google_pubsub_topic" "tf_pubsub_topic" {
  name = "tf-pubsub-topic"

  labels = {
    env = "terraform"
  }
}

resource "google_pubsub_subscription" "tf_pubsub_subscription" {
  name                 = "tf-pubsub-subscription"
  topic                = google_pubsub_topic.tf_pubsub_topic.id
  ack_deadline_seconds = 20

  labels = {
    env = "terraform"
  }
}

# ---------- Cloud Build ----------

resource "google_cloudbuild_trigger" "tf_cloudbuild_trigger" {
  name     = "tf-cloudbuild-trigger"
  location = "us-central1"
  filename = "cloudbuild.yaml"

  trigger_template {
    branch_name = "main"
    repo_name   = "tf-repo"
  }

  substitutions = {
    _ENV = "terraform"
  }
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

# ---------- Cloud Logging ----------

resource "google_logging_project_sink" "tf_log_sink" {
  name                   = "tf-log-sink"
  destination            = "storage.googleapis.com/${google_storage_bucket.tf_bucket.name}"
  filter                 = "resource.type=\"global\""
  unique_writer_identity = true
}

resource "google_logging_metric" "tf_log_metric" {
  name   = "tf-log-metric"
  filter = "resource.type=\"global\" AND severity>=ERROR"

  metric_descriptor {
    metric_kind = "DELTA"
    value_type  = "INT64"
  }
}

# ---------- Data SaaS (BigQuery + Firestore) ----------

resource "google_bigquery_dataset" "tf_bq_dataset" {
  dataset_id = "tf_test_dataset"
  location   = "US"

  labels = {
    env = "terraform"
  }
}

resource "google_bigquery_table" "tf_bq_table" {
  dataset_id          = google_bigquery_dataset.tf_bq_dataset.dataset_id
  table_id            = "events"
  deletion_protection = false

  schema = jsonencode([
    {
      name = "id"
      type = "STRING"
      mode = "REQUIRED"
    },
    {
      name = "kind"
      type = "STRING"
      mode = "NULLABLE"
    }
  ])
}

resource "google_firestore_document" "tf_firestore_doc" {
  project     = "test-project"
  collection  = "tf-users"
  document_id = "alice"
  fields      = "{\"team\":{\"stringValue\":\"platform\"},\"role\":{\"stringValue\":\"admin\"}}"
}

# ---------- Secret Manager ----------

resource "google_secret_manager_secret" "tf_secret" {
  secret_id       = "tf-test-secret"
  deletion_policy = "DELETE"

  labels = {
    env = var.secret_label_env
  }

  replication {
    auto {}
  }
}

resource "google_secret_manager_secret_version" "tf_secret_v1" {
  secret          = google_secret_manager_secret.tf_secret.id
  secret_data     = "tf-test-secret-payload"
  deletion_policy = "DELETE"
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

output "cloudfunctions2_function_id" {
  value = google_cloudfunctions2_function.tf_gcfv2_function.id
}

output "eventarc_trigger_id" {
  value = google_eventarc_trigger.tf_eventarc_trigger.id
}

output "pubsub_topic_id" {
  value = google_pubsub_topic.tf_pubsub_topic.id
}

output "pubsub_subscription_id" {
  value = google_pubsub_subscription.tf_pubsub_subscription.id
}

output "cloudbuild_trigger_id" {
  value = google_cloudbuild_trigger.tf_cloudbuild_trigger.id
}

output "storage_bucket_url" {
  value = google_storage_bucket.tf_bucket.url
}

output "logging_project_sink_id" {
  value = google_logging_project_sink.tf_log_sink.id
}

output "logging_metric_id" {
  value = google_logging_metric.tf_log_metric.id
}

output "secret_version_id" {
  value = google_secret_manager_secret_version.tf_secret_v1.id
}

output "secret_label_env" {
  value = google_secret_manager_secret.tf_secret.labels.env
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

output "bigquery_table_id" {
  value = google_bigquery_table.tf_bq_table.id
}

output "firestore_document_name" {
  value = google_firestore_document.tf_firestore_doc.id
}
