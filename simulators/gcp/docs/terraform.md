# Using the GCP simulator with Terraform

## Prerequisites

- Terraform installed (`terraform version`)
- Simulator running on `http://localhost:4567`

## Provider configuration

Use the official `hashicorp/google` provider with custom endpoint overrides:

```hcl
terraform {
  required_providers {
    google = {
      source = "hashicorp/google"
    }
  }
}

provider "google" {
  project = "my-project"
  region  = "us-central1"

  access_token          = "test-token"
  user_project_override = false

  # Point each service at the simulator
  compute_custom_endpoint               = "http://localhost:4567/compute/v1/"
  dns_custom_endpoint                   = "http://localhost:4567/dns/v1/"
  cloud_run_custom_endpoint             = "http://localhost:4567/"
  cloudfunctions_custom_endpoint        = "http://localhost:4567/"
  storage_custom_endpoint               = "http://localhost:4567/storage/v1/"
  artifact_registry_custom_endpoint     = "http://localhost:4567/"
  logging_custom_endpoint               = "http://localhost:4567/"
  service_usage_custom_endpoint         = "http://localhost:4567/"
  iam_custom_endpoint                   = "http://localhost:4567/"
  vpc_access_custom_endpoint            = "http://localhost:4567/"
}
```

Note the trailing `/` on endpoint URLs — the Google provider appends API paths directly.

## Example resources

```hcl
resource "google_compute_network" "main" {
  name                    = "my-network"
  auto_create_subnetworks = false
}

resource "google_compute_subnetwork" "main" {
  name          = "my-subnet"
  ip_cidr_range = "10.0.0.0/24"
  network       = google_compute_network.main.id
  region        = "us-central1"
}

resource "google_dns_managed_zone" "main" {
  name     = "my-zone"
  dns_name = "example.com."
}

resource "google_dns_record_set" "a" {
  name         = "www.example.com."
  managed_zone = google_dns_managed_zone.main.name
  type         = "A"
  ttl          = 300
  rrdatas      = ["10.0.0.1"]
}

resource "google_service_account" "main" {
  account_id   = "my-sa"
  display_name = "My Service Account"
}

resource "google_artifact_registry_repository" "main" {
  repository_id = "my-repo"
  location      = "us-central1"
  format        = "DOCKER"
}

resource "google_storage_bucket" "main" {
  name     = "my-bucket"
  location = "US"
}

resource "google_vpc_access_connector" "main" {
  name          = "my-connector"
  region        = "us-central1"
  ip_cidr_range = "10.8.0.0/28"
  network       = google_compute_network.main.name
}
```

## Running

Pass the simulator endpoint via a variable:

```sh
terraform init
terraform apply -auto-approve -var="endpoint=http://localhost:4567"
terraform destroy -auto-approve -var="endpoint=http://localhost:4567"
```

With a `variables.tf`:

```hcl
variable "endpoint" {
  description = "Simulator endpoint URL"
  type        = string
  default     = "http://localhost:4567"
}
```

Then use `var.endpoint` as the base for custom endpoint URLs in the provider block:

```hcl
compute_custom_endpoint = "${var.endpoint}/compute/v1/"
dns_custom_endpoint     = "${var.endpoint}/dns/v1/"
```

## Supported resources

| Category | Resources |
|----------|-----------|
| Compute | `google_compute_network`, `google_compute_subnetwork` |
| DNS | `google_dns_managed_zone`, `google_dns_record_set` |
| IAM | `google_service_account`, `google_project_iam_member` |
| Cloud Run | `google_cloud_run_v2_job` |
| Cloud Functions | `google_cloudfunctions2_function` |
| Storage | `google_storage_bucket` |
| Artifact Registry | `google_artifact_registry_repository` |
| VPC Access | `google_vpc_access_connector` |
| Service Usage | `google_project_service` |

## Notes

- All state is in-memory and resets when the simulator restarts. Terraform state files will become stale after a restart.
- Authentication is not validated — any `access_token` value will work.
- Long-running operations return immediately with `done: true`, so Terraform never blocks on polling.
