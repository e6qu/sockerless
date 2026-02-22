output "project_id" {
  value = module.sockerless_gcf.project_id
}

output "region" {
  value = module.sockerless_gcf.region
}

output "artifact_registry_url" {
  value = module.sockerless_gcf.artifact_registry_repository_url
}

output "service_account_email" {
  value = module.sockerless_gcf.service_account_email
}

# Print the env vars needed to run the backend
output "backend_env" {
  value = <<-EOT
    export SOCKERLESS_GCF_PROJECT=${module.sockerless_gcf.project_id}
    export SOCKERLESS_GCF_REGION=${module.sockerless_gcf.region}
    export SOCKERLESS_GCF_SERVICE_ACCOUNT=${module.sockerless_gcf.service_account_email}
    export SOCKERLESS_CALLBACK_URL=http://<YOUR_BACKEND_HOST>:9100
  EOT
}
