# ---------------------------------------------------------------------------
# Resource Group
# ---------------------------------------------------------------------------
output "resource_group_name" {
  description = "Name of the resource group containing all ACA resources"
  value       = local.resource_group_name
}

output "location" {
  description = "Azure region where resources are deployed"
  value       = var.location
}

# ---------------------------------------------------------------------------
# Container Apps Environment
# ---------------------------------------------------------------------------
output "managed_environment_name" {
  description = "Name of the Container Apps managed environment"
  value       = azurerm_container_app_environment.this.name
}

output "managed_environment_id" {
  description = "Resource ID of the Container Apps managed environment"
  value       = azurerm_container_app_environment.this.id
}

# ---------------------------------------------------------------------------
# Networking
# ---------------------------------------------------------------------------
output "vnet_id" {
  description = "Resource ID of the virtual network"
  value       = azurerm_virtual_network.this.id
}

output "subnet_id" {
  description = "Resource ID of the Container Apps subnet"
  value       = azurerm_subnet.container_apps.id
}

# ---------------------------------------------------------------------------
# Log Analytics
# ---------------------------------------------------------------------------
output "log_analytics_workspace_id" {
  description = "Resource ID of the Log Analytics workspace"
  value       = azurerm_log_analytics_workspace.this.id
}

output "log_analytics_workspace_name" {
  description = "Name of the Log Analytics workspace"
  value       = azurerm_log_analytics_workspace.this.name
}

output "log_analytics_workspace_key" {
  description = "Primary shared key of the Log Analytics workspace"
  value       = azurerm_log_analytics_workspace.this.primary_shared_key
  sensitive   = true
}

# ---------------------------------------------------------------------------
# Storage
# ---------------------------------------------------------------------------
output "storage_account_name" {
  description = "Name of the storage account for Azure Files volume mounts"
  value       = azurerm_storage_account.this.name
}

output "storage_account_id" {
  description = "Resource ID of the storage account"
  value       = azurerm_storage_account.this.id
}

output "storage_account_key" {
  description = "Primary access key for the storage account"
  value       = azurerm_storage_account.this.primary_access_key
  sensitive   = true
}

output "file_share_name" {
  description = "Name of the default Azure Files share for volume mounts"
  value       = azurerm_storage_share.this.name
}

# ---------------------------------------------------------------------------
# Azure Container Registry
# ---------------------------------------------------------------------------
output "acr_login_server" {
  description = "Login server URL for the Azure Container Registry"
  value       = azurerm_container_registry.this.login_server
}

output "acr_name" {
  description = "Name of the Azure Container Registry"
  value       = azurerm_container_registry.this.name
}

# ---------------------------------------------------------------------------
# Managed Identity
# ---------------------------------------------------------------------------
output "managed_identity_id" {
  description = "Resource ID of the user-assigned managed identity"
  value       = azurerm_user_assigned_identity.this.id
}

output "managed_identity_client_id" {
  description = "Client ID of the user-assigned managed identity"
  value       = azurerm_user_assigned_identity.this.client_id
}

output "managed_identity_principal_id" {
  description = "Principal ID of the user-assigned managed identity"
  value       = azurerm_user_assigned_identity.this.principal_id
}

# ---------------------------------------------------------------------------
# DNS Zone
# ---------------------------------------------------------------------------
output "dns_zone_name" {
  description = "Name of the private DNS zone for service discovery"
  value       = azurerm_private_dns_zone.this.name
}

output "dns_zone_id" {
  description = "Resource ID of the private DNS zone"
  value       = azurerm_private_dns_zone.this.id
}
