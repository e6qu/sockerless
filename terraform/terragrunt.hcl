# Root terragrunt configuration for Sockerless
# All child configurations inherit from this

locals {
  # Parse the path to extract environment info
  # Expected path: environments/<backend>/<env>/terragrunt.hcl
  path_parts = split("/", path_relative_to_include())
  backend_name = local.path_parts[1]
  env_name     = local.path_parts[2]

  # Common tags/labels for all resources
  common_tags = {
    project    = "sockerless"
    managed-by = "terragrunt"
  }
}

# Generate provider versions
generate "versions" {
  path      = "versions_override.tf"
  if_exists = "overwrite_terragrunt"
  contents  = <<EOF
terraform {
  required_version = ">= 1.5"
}
EOF
}

# Remote state configuration is defined per-environment
# (AWS uses S3, GCP uses GCS, Azure uses azurerm blob)
