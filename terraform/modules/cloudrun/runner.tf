# =============================================================================
# Runner-task workspace
# =============================================================================
#
# GCS bucket backing the github-runner-dispatcher-gcp's spawned runner
# Cloud Run Job's `/tmp/runner-work` and `/opt/runner/externals` shared
# volumes (BUG-909, Phase 122d). Cloud Run Jobs natively support
# `Volume{Gcs{Bucket}}`; the dispatcher mounts this bucket on the
# runner-task and sets `SOCKERLESS_GCP_SHARED_VOLUMES=runner-workspace=
# /tmp/runner-work=<bucket>,runner-externals=/opt/runner/externals=
# <bucket>` so the in-image sockerless backend's bind-mount translator
# routes sub-task containers to the same bucket.
#
# Symmetric with `terraform/modules/ecs/runner.tf` and
# `terraform/modules/lambda/runner.tf` (EFS-backed) — same shape, same
# 1-day lifecycle, same `force_destroy` for clean teardown.

resource "google_storage_bucket" "runner_workspace" {
  project                     = var.project_id
  name                        = "${var.project_id}-runner-workspace"
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

# Runner SA full read/write on the workspace bucket so the github-
# runner inside the runner-task can write workflow artefacts and the
# spawned sub-task containers can read/write the shared workspace.
resource "google_storage_bucket_iam_member" "runner_workspace_admin" {
  bucket = google_storage_bucket.runner_workspace.name
  role   = "roles/storage.admin"
  member = "serviceAccount:${google_service_account.runner.email}"
}
