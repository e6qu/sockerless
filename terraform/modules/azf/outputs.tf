# =============================================================================
# Resource Group
# =============================================================================

output "resource_group_name" {
  description = "Name of the resource group containing all Azure Functions resources"
  value       = local.resource_group_name
}

output "location" {
  description = "Azure region where resources are deployed"
  value       = local.location
}

# =============================================================================
# Storage Account
# =============================================================================

output "storage_account_name" {
  description = "Name of the storage account used by Azure Functions runtime"
  value       = azurerm_storage_account.main.name
}

# =============================================================================
# App Service Plan
# =============================================================================

output "app_service_plan_id" {
  description = "ID of the App Service Plan for Azure Functions"
  value       = azurerm_service_plan.main.id
}

# =============================================================================
# Azure Container Registry
# =============================================================================

output "acr_login_server" {
  description = "Login server URL of the Azure Container Registry"
  value       = azurerm_container_registry.main.login_server
}

# =============================================================================
# Managed Identity
# =============================================================================

output "managed_identity_id" {
  description = "ID of the user-assigned managed identity"
  value       = azurerm_user_assigned_identity.main.id
}

output "managed_identity_client_id" {
  description = "Client ID of the user-assigned managed identity"
  value       = azurerm_user_assigned_identity.main.client_id
}

# =============================================================================
# Monitoring
# =============================================================================

output "application_insights_connection_string" {
  description = "Connection string for Application Insights"
  value       = azurerm_application_insights.main.connection_string
  sensitive   = true
}

output "log_analytics_workspace_id" {
  description = "ID of the Log Analytics workspace"
  value       = azurerm_log_analytics_workspace.main.id
}
