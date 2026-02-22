# Computed values and merged labels for consistent resource configuration.
#
# GCP labels must be lowercase with only letters, numbers, hyphens,
# and underscores. The common_labels local merges standard project
# labels with any additional labels provided via the labels variable.

locals {
  # Resource naming prefix
  name_prefix = "${var.project_name}-${var.environment}"

  # Standard labels applied to all resources that support labeling
  common_labels = merge(
    {
      project     = lower(var.project_name)
      environment = lower(var.environment)
      component   = "cloudrun"
      managed-by  = "terraform"
    },
    { for k, v in var.labels : lower(k) => lower(v) }
  )
}
