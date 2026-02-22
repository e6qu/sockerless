output "resource_group_name" {
  value = module.sockerless_azf.resource_group_name
}

output "storage_account_name" {
  value = module.sockerless_azf.storage_account_name
}

output "app_service_plan_id" {
  value = module.sockerless_azf.app_service_plan_id
}

output "acr_login_server" {
  value = module.sockerless_azf.acr_login_server
}

output "log_analytics_workspace_id" {
  value = module.sockerless_azf.log_analytics_workspace_id
}

# Print the env vars needed to run the backend
output "backend_env" {
  sensitive = true
  value     = <<-EOT
    export SOCKERLESS_AZF_SUBSCRIPTION_ID=$(az account show --query id -o tsv)
    export SOCKERLESS_AZF_RESOURCE_GROUP=${module.sockerless_azf.resource_group_name}
    export SOCKERLESS_AZF_LOCATION=${module.sockerless_azf.location}
    export SOCKERLESS_AZF_STORAGE_ACCOUNT=${module.sockerless_azf.storage_account_name}
    export SOCKERLESS_AZF_LOG_ANALYTICS_WORKSPACE=${module.sockerless_azf.log_analytics_workspace_id}
    export SOCKERLESS_CALLBACK_URL=http://<YOUR_BACKEND_HOST>:9100
  EOT
}
