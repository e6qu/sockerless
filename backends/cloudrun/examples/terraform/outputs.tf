output "project_id" {
  value = module.sockerless_cloudrun.project_id
}

output "region" {
  value = module.sockerless_cloudrun.region
}

output "vpc_connector_name" {
  value = module.sockerless_cloudrun.vpc_connector_name
}

output "artifact_registry_url" {
  value = module.sockerless_cloudrun.artifact_registry_repository_url
}

output "service_account_email" {
  value = module.sockerless_cloudrun.service_account_email
}

# Print the env vars needed to run the backend
output "backend_env" {
  value = <<-EOT
    export SOCKERLESS_GCR_PROJECT=${module.sockerless_cloudrun.project_id}
    export SOCKERLESS_GCR_REGION=${module.sockerless_cloudrun.region}
    export SOCKERLESS_GCR_VPC_CONNECTOR=${module.sockerless_cloudrun.vpc_connector_id}
    export SOCKERLESS_GCR_LOG_ID=sockerless
  EOT
}
