locals {
  # Consistent tags applied to all resources
  common_tags = merge(var.tags, {
    environment = var.environment
    project     = var.project_name
    managed-by  = "terraform"
  })

  # Resource group name: either created or provided
  resource_group_name = var.create_resource_group ? azurerm_resource_group.this[0].name : data.azurerm_resource_group.existing[0].name
  resource_group_id   = var.create_resource_group ? azurerm_resource_group.this[0].id : data.azurerm_resource_group.existing[0].id
}

# ---------------------------------------------------------------------------
# Random suffix for globally unique storage account name
# ---------------------------------------------------------------------------
resource "random_string" "storage_suffix" {
  length  = 6
  special = false
  upper   = false
}

# ---------------------------------------------------------------------------
# Resource Group
# ---------------------------------------------------------------------------
resource "azurerm_resource_group" "this" {
  count    = var.create_resource_group ? 1 : 0
  name     = "${var.name_prefix}-${var.environment}-rg"
  location = var.location
  tags     = local.common_tags
}

data "azurerm_resource_group" "existing" {
  count = var.create_resource_group ? 0 : 1
  name  = var.resource_group_name
}

# ---------------------------------------------------------------------------
# Virtual Network
# ---------------------------------------------------------------------------
resource "azurerm_virtual_network" "this" {
  name                = "${var.name_prefix}-${var.environment}-vnet"
  location            = var.location
  resource_group_name = local.resource_group_name
  address_space       = var.vnet_address_space
  tags                = local.common_tags
}

# ---------------------------------------------------------------------------
# Subnet — delegated to Microsoft.App/environments for Container Apps
# Minimum /23 CIDR range is required for Container Apps Environment VNet
# integration.
# ---------------------------------------------------------------------------
resource "azurerm_subnet" "container_apps" {
  name                 = "${var.name_prefix}-${var.environment}-aca-subnet"
  resource_group_name  = local.resource_group_name
  virtual_network_name = azurerm_virtual_network.this.name
  address_prefixes     = [var.subnet_address_prefix]

  delegation {
    name = "container-apps-delegation"

    service_delegation {
      name    = "Microsoft.App/environments"
      actions = ["Microsoft.Network/virtualNetworks/subnets/join/action"]
    }
  }
}

# ---------------------------------------------------------------------------
# Network Security Group
# ---------------------------------------------------------------------------
resource "azurerm_network_security_group" "this" {
  name                = "${var.name_prefix}-${var.environment}-nsg"
  location            = var.location
  resource_group_name = local.resource_group_name
  tags                = local.common_tags

  # Allow inbound TCP 9111 from VNet for sockerless-agent traffic
  security_rule {
    name                       = "AllowAgentPortFromVNet"
    priority                   = 100
    direction                  = "Inbound"
    access                     = "Allow"
    protocol                   = "Tcp"
    source_port_range          = "*"
    destination_port_range     = "9111"
    source_address_prefix      = "VirtualNetwork"
    destination_address_prefix = "VirtualNetwork"
  }

  # Allow outbound HTTPS for Azure SDK calls, registry pulls, etc.
  security_rule {
    name                       = "AllowOutboundHTTPS"
    priority                   = 100
    direction                  = "Outbound"
    access                     = "Allow"
    protocol                   = "Tcp"
    source_port_range          = "*"
    destination_port_range     = "443"
    source_address_prefix      = "VirtualNetwork"
    destination_address_prefix = "Internet"
  }

  # Deny all other inbound from Internet
  security_rule {
    name                       = "DenyAllInboundFromInternet"
    priority                   = 4096
    direction                  = "Inbound"
    access                     = "Deny"
    protocol                   = "*"
    source_port_range          = "*"
    destination_port_range     = "*"
    source_address_prefix      = "Internet"
    destination_address_prefix = "VirtualNetwork"
  }
}

resource "azurerm_subnet_network_security_group_association" "this" {
  subnet_id                 = azurerm_subnet.container_apps.id
  network_security_group_id = azurerm_network_security_group.this.id
}

# ---------------------------------------------------------------------------
# Log Analytics Workspace
# ---------------------------------------------------------------------------
resource "azurerm_log_analytics_workspace" "this" {
  name                = "${var.name_prefix}-${var.environment}-logs"
  location            = var.location
  resource_group_name = local.resource_group_name
  sku                 = "PerGB2018"
  retention_in_days   = var.log_retention_days
  tags                = local.common_tags
}

# ---------------------------------------------------------------------------
# Container Apps Environment
# Linked to the delegated subnet and Log Analytics workspace.
# Consumption workload profile minimizes idle cost.
# ---------------------------------------------------------------------------
resource "azurerm_container_app_environment" "this" {
  name                       = "${var.name_prefix}-${var.environment}-env"
  location                   = var.location
  resource_group_name        = local.resource_group_name
  log_analytics_workspace_id = azurerm_log_analytics_workspace.this.id
  infrastructure_subnet_id   = azurerm_subnet.container_apps.id

  workload_profile {
    name                  = "Consumption"
    workload_profile_type = "Consumption"
  }

  tags = local.common_tags
}

# ---------------------------------------------------------------------------
# Storage Account — globally unique name using random suffix
# Used for Azure Files volume mounts in Container Apps Jobs.
# ---------------------------------------------------------------------------
resource "azurerm_storage_account" "this" {
  name                     = "${var.name_prefix}sa${random_string.storage_suffix.result}"
  resource_group_name      = local.resource_group_name
  location                 = var.location
  account_tier             = "Standard"
  account_replication_type = var.storage_account_replication_type
  min_tls_version          = "TLS1_2"

  tags = local.common_tags
}

# ---------------------------------------------------------------------------
# Azure Files Share — default share for volume mounts
# ---------------------------------------------------------------------------
resource "azurerm_storage_share" "this" {
  name                 = "sockerless-volumes"
  storage_account_name = azurerm_storage_account.this.name
  quota                = var.file_share_quota
}

# ---------------------------------------------------------------------------
# Azure Container Registry
# ---------------------------------------------------------------------------
resource "azurerm_container_registry" "this" {
  name                = "${var.name_prefix}${var.environment}acr"
  resource_group_name = local.resource_group_name
  location            = var.location
  sku                 = var.acr_sku
  admin_enabled       = false

  tags = local.common_tags
}

# ---------------------------------------------------------------------------
# User-Assigned Managed Identity
# Used by the ACA backend for authenticating to Azure resources.
# ---------------------------------------------------------------------------
resource "azurerm_user_assigned_identity" "this" {
  name                = "${var.name_prefix}-${var.environment}-identity"
  location            = var.location
  resource_group_name = local.resource_group_name
  tags                = local.common_tags
}

# ---------------------------------------------------------------------------
# RBAC Role Assignments for the Managed Identity
# ---------------------------------------------------------------------------

# Contributor on Container Apps Environment — allows creating/managing jobs
resource "azurerm_role_assignment" "identity_contributor_env" {
  scope                = azurerm_container_app_environment.this.id
  role_definition_name = "Contributor"
  principal_id         = azurerm_user_assigned_identity.this.principal_id
}

# Storage Blob Data Contributor on the storage account — allows volume operations
resource "azurerm_role_assignment" "identity_storage_contributor" {
  scope                = azurerm_storage_account.this.id
  role_definition_name = "Storage Blob Data Contributor"
  principal_id         = azurerm_user_assigned_identity.this.principal_id
}

# AcrPull on the ACR — allows pulling container images
resource "azurerm_role_assignment" "identity_acr_pull" {
  scope                = azurerm_container_registry.this.id
  role_definition_name = "AcrPull"
  principal_id         = azurerm_user_assigned_identity.this.principal_id
}

# Monitoring Reader on the resource group — allows reading logs and metrics
resource "azurerm_role_assignment" "identity_monitoring_reader" {
  scope                = local.resource_group_id
  role_definition_name = "Monitoring Reader"
  principal_id         = azurerm_user_assigned_identity.this.principal_id
}

# ---------------------------------------------------------------------------
# Private DNS Zone — for service discovery within the VNet
# ---------------------------------------------------------------------------
resource "azurerm_private_dns_zone" "this" {
  name                = "sockerless.internal"
  resource_group_name = local.resource_group_name
  tags                = local.common_tags
}

resource "azurerm_private_dns_zone_virtual_network_link" "this" {
  name                  = "${var.name_prefix}-${var.environment}-dns-link"
  resource_group_name   = local.resource_group_name
  private_dns_zone_name = azurerm_private_dns_zone.this.name
  virtual_network_id    = azurerm_virtual_network.this.id
  registration_enabled  = true
  tags                  = local.common_tags
}
