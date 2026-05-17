# simulator-gcp-terraform-tests

Integration tests that run `terraform apply` and `terraform destroy` against the GCP simulator. Verifies that the simulator implements enough of the GCP API surface for the Terraform Google provider to provision and tear down resources.

Resources covered:
- `google_compute_network` + `google_compute_disk` + `google_compute_subnetwork` + `google_compute_firewall`
- `google_dns_managed_zone` (public + private)
- `google_artifact_registry_repository` (Docker)
- `google_cloud_run_v2_service` + `google_cloud_run_v2_job`
- `google_storage_bucket` + `google_storage_bucket_object`
- `google_secret_manager_secret` + `google_secret_manager_secret_version`

Resources NOT yet covered (filed as follow-ups): `google_service_account` + IAM binding/member (terraform-provider-google's IAM resources don't honour `iam_custom_endpoint`, hit real iam.googleapis.com); `google_cloudfunctions2_function` (build_config requires a real source archive); `google_compute_instance` + `_instance_template`; `google_cloudbuild_trigger`; `google_logging_project_sink` + `_metric`; `google_pubsub_topic` + `_subscription`.

## Running

```sh
cd simulators/gcp/terraform-tests
go test -v ./...
```

The test harness (`helpers_test.go`) handles simulator binary build, port allocation, server startup, Terraform init/apply/destroy, and shutdown. No external services required.

## Prerequisites

- Go 1.23+
- `terraform` CLI installed and on `PATH`
- The `simulators/gcp/` parent module (built automatically by `TestMain`)

## How it works

1. `TestMain` builds the GCP simulator binary and starts it on a free port
2. Tests write Terraform configurations to a temp directory
3. `terraform init` downloads the Google provider
4. `terraform apply -auto-approve` provisions resources against the simulator
5. Test assertions verify the Terraform state
6. `terraform destroy -auto-approve` tears down resources
