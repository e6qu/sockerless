# =============================================================================
# Azure Functions Terraform Module
# =============================================================================
#
# Provisions Azure infrastructure required by the Sockerless Azure Functions
# backend. This includes:
#   - Resource group (optional, can use existing)
#   - Storage account (required by Azure Functions runtime)
#   - App Service Plan (Linux consumption or premium)
#   - Azure Container Registry for container images
#   - User-assigned managed identity with RBAC roles
#   - Log Analytics workspace and Application Insights
#
# The storage account is required by Azure Functions for triggers, bindings,
# and internal state. The storage account name must be globally unique, so
# a random suffix is appended.
#
# Prerequisites:
#   - AzureRM provider configured with appropriate credentials
#   - Terraform >= 1.5
#
# Usage:
#   module "azf" {
#     source      = "../../modules/azf"
#     environment = "test"
#   }
# =============================================================================

locals {
  name_prefix = "${var.name_prefix}-${var.environment}"

  # Resource group name: use provided name or generate one
  resource_group_name = var.create_resource_group ? azurerm_resource_group.main[0].name : var.resource_group_name

  # Resource group location: use provided location
  location = var.location

  common_tags = merge(var.tags, {
    project     = var.project_name
    environment = var.environment
    component   = "azf"
    managed-by  = "terraform"
  })
}

# =============================================================================
# Sockerless runtime sweep
# =============================================================================
# Sockerless creates Function Apps at runtime inside the resource
# group; they're not in this module's state. On destroy, sweep every
# sockerless-tagged Function App before the storage / ACR / service
# plan teardown — Function App removal is gated on these resources and
# Azure surfaces confusing "in use" errors otherwise. Symmetric with
# the AWS / GCP module sweeps per the project teardown rule.
resource "null_resource" "sockerless_runtime_sweep" {
  triggers = {
    rg = local.resource_group_name
  }

  provisioner "local-exec" {
    when    = destroy
    command = <<-EOT
      set -eu
      rg='${self.triggers.rg}'
      echo "sockerless-azf-sweep: rg=$rg"

      for app in $(az functionapp list --resource-group "$rg" --query "[?tags.\"sockerless-managed\"=='true'].name" -o tsv 2>/dev/null); do
        [ -z "$app" ] && continue
        az functionapp delete --resource-group "$rg" --name "$app" >/dev/null 2>&1 || true
      done
    EOT
  }
}

# =============================================================================
# Resource Group
# =============================================================================

resource "azurerm_resource_group" "main" {
  count = var.create_resource_group ? 1 : 0

  name     = "${local.name_prefix}-rg"
  location = var.location

  tags = local.common_tags
}

# =============================================================================
# Random String for Storage Account Name Uniqueness
# =============================================================================

# Storage account names must be globally unique, 3-24 characters,
# lowercase letters and numbers only.
resource "random_string" "storage_suffix" {
  length  = 8
  special = false
  upper   = false
}

# =============================================================================
# Storage Account
# =============================================================================

resource "azurerm_storage_account" "main" {
  name                     = "${replace(var.name_prefix, "-", "")}${random_string.storage_suffix.result}"
  resource_group_name      = local.resource_group_name
  location                 = local.location
  account_tier             = "Standard"
  account_replication_type = var.storage_replication_type

  # Security: enforce HTTPS-only access
  enable_https_traffic_only = true

  tags = merge(local.common_tags, {
    purpose = "azure-functions-runtime"
  })

  depends_on = [azurerm_resource_group.main]
}

# =============================================================================
# App Service Plan
# =============================================================================

resource "azurerm_service_plan" "main" {
  name                = "${local.name_prefix}-plan"
  resource_group_name = local.resource_group_name
  location            = local.location
  os_type             = "Linux"
  sku_name            = var.app_service_plan_sku

  tags = local.common_tags

  depends_on = [azurerm_resource_group.main]
}

# =============================================================================
# Azure Container Registry
# =============================================================================

resource "azurerm_container_registry" "main" {
  name                = "${replace(var.name_prefix, "-", "")}acr"
  resource_group_name = local.resource_group_name
  location            = local.location
  sku                 = var.acr_sku
  admin_enabled       = false

  tags = local.common_tags

  depends_on = [azurerm_resource_group.main]
}

# =============================================================================
# User-Assigned Managed Identity
# =============================================================================

resource "azurerm_user_assigned_identity" "main" {
  name                = "${local.name_prefix}-identity"
  resource_group_name = local.resource_group_name
  location            = local.location

  tags = local.common_tags

  depends_on = [azurerm_resource_group.main]
}

# =============================================================================
# RBAC Role Assignments
# =============================================================================

# AcrPull — allow the managed identity to pull images from ACR
resource "azurerm_role_assignment" "acr_pull" {
  scope                = azurerm_container_registry.main.id
  role_definition_name = "AcrPull"
  principal_id         = azurerm_user_assigned_identity.main.principal_id
}

# AcrPush — allow the in-image sockerless backend to push runtime
# image builds (per-arch + manifest list) produced by ACR Tasks.
resource "azurerm_role_assignment" "acr_push" {
  scope                = azurerm_container_registry.main.id
  role_definition_name = "AcrPush"
  principal_id         = azurerm_user_assigned_identity.main.principal_id
}

# Contributor on the ACR — required to submit ACR Tasks (the
# sockerless-sanctioned image builder for Azure, analogous to
# AWS CodeBuild and GCP Cloud Build).
resource "azurerm_role_assignment" "acr_contributor" {
  scope                = azurerm_container_registry.main.id
  role_definition_name = "Contributor"
  principal_id         = azurerm_user_assigned_identity.main.principal_id
}

# Storage Blob Data Contributor — allow the managed identity to access
# the storage account used by Azure Functions runtime
resource "azurerm_role_assignment" "storage_blob_contributor" {
  scope                = azurerm_storage_account.main.id
  role_definition_name = "Storage Blob Data Contributor"
  principal_id         = azurerm_user_assigned_identity.main.principal_id
}

# ACR cache rule for Docker Hub — Azure analogue of the GCP
# `docker-hub` Artifact Registry remote-proxy. Sockerless rewrites
# `docker.io/library/alpine:latest` to
# `<acr>.azurecr.io/docker-hub/library/alpine:latest`. Required for
# AZF sub-task containers that reference any Docker Hub base image.
#
# Cache rules require Standard or Premium ACR SKU.
resource "azurerm_container_registry_cache_rule" "docker_hub" {
  count                 = var.create_docker_hub_cache_rule ? 1 : 0
  name                  = "docker-hub"
  container_registry_id = azurerm_container_registry.main.id
  target_repo           = "docker-hub/*"
  source_repo           = "docker.io/*"
}

# Monitoring Reader — allow the managed identity to read monitoring data
resource "azurerm_role_assignment" "monitoring_reader" {
  scope                = var.create_resource_group ? azurerm_resource_group.main[0].id : "/subscriptions/${data.azurerm_subscription.current.subscription_id}/resourceGroups/${var.resource_group_name}"
  role_definition_name = "Monitoring Reader"
  principal_id         = azurerm_user_assigned_identity.main.principal_id
}

# =============================================================================
# Data Sources
# =============================================================================

data "azurerm_subscription" "current" {}

# =============================================================================
# Log Analytics Workspace
# =============================================================================

resource "azurerm_log_analytics_workspace" "main" {
  name                = "${local.name_prefix}-law"
  resource_group_name = local.resource_group_name
  location            = local.location
  sku                 = "PerGB2018"
  retention_in_days   = var.log_retention_days

  tags = local.common_tags

  depends_on = [azurerm_resource_group.main]
}

# =============================================================================
# Application Insights
# =============================================================================

resource "azurerm_application_insights" "main" {
  name                = "${local.name_prefix}-appinsights"
  resource_group_name = local.resource_group_name
  location            = local.location
  workspace_id        = azurerm_log_analytics_workspace.main.id
  application_type    = "other"

  tags = local.common_tags

  depends_on = [azurerm_resource_group.main]
}
