output "resource_group_name" {
  value = module.sockerless_aca.resource_group_name
}

output "managed_environment_name" {
  value = module.sockerless_aca.managed_environment_name
}

output "log_analytics_workspace_id" {
  value = module.sockerless_aca.log_analytics_workspace_id
}

output "acr_login_server" {
  value = module.sockerless_aca.acr_login_server
}

output "storage_account_name" {
  value = module.sockerless_aca.storage_account_name
}

# Print the env vars needed to run the backend
output "backend_env" {
  sensitive = true
  value     = <<-EOT
    export SOCKERLESS_ACA_SUBSCRIPTION_ID=$(az account show --query id -o tsv)
    export SOCKERLESS_ACA_RESOURCE_GROUP=${module.sockerless_aca.resource_group_name}
    export SOCKERLESS_ACA_ENVIRONMENT=${module.sockerless_aca.managed_environment_name}
    export SOCKERLESS_ACA_LOCATION=${module.sockerless_aca.location}
    export SOCKERLESS_ACA_LOG_ANALYTICS_WORKSPACE=${module.sockerless_aca.log_analytics_workspace_id}
    export SOCKERLESS_ACA_STORAGE_ACCOUNT=${module.sockerless_aca.storage_account_name}
  EOT
}
