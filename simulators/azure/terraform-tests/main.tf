terraform {
  required_providers {
    azurestack = {
      source  = "hashicorp/azurestack"
      version = "~> 1.0"
    }
    azurerm = {
      source = "hashicorp/azurerm"
    }
  }
}

provider "azurestack" {
  arm_endpoint    = var.endpoint
  client_id       = "test-client-id"
  client_secret   = "test-client-secret"
  tenant_id       = "11111111-1111-1111-1111-111111111111"
  subscription_id = "00000000-0000-0000-0000-000000000001"

  skip_provider_registration = true

  features {}
}

# azurerm provider against the sim — drives ACR + Container Apps + Function
# Apps + App Insights + Managed Identity + Private DNS + Key Vault data-
# plane, which azurestack doesn't expose. The sim ships /metadata/endpoints
# (api-version=2022-09-01) + /<tenant>/oauth2/v2.0/token + JWKS + OpenID
# discovery; together these let azurerm bootstrap its cloud config and
# auth without ever reaching real Azure.
provider "azurerm" {
  client_id       = "test-client-id"
  client_secret   = "test-client-secret"
  tenant_id       = "11111111-1111-1111-1111-111111111111"
  subscription_id = "00000000-0000-0000-0000-000000000001"

  metadata_host = trimprefix(trimprefix(var.endpoint, "https://"), "http://")

  skip_provider_registration   = true
  resource_provider_registrations = "none"

  features {}
}

# ---------- Resource group (kept; foundation for everything below) ----------

resource "azurestack_resource_group" "main" {
  name     = "tf-test-rg"
  location = "eastus"
}

# ---------- Virtual Network + Subnet ----------

resource "azurestack_virtual_network" "main" {
  name                = "tf-test-vnet"
  resource_group_name = azurestack_resource_group.main.name
  location            = azurestack_resource_group.main.location

  address_space = ["10.0.0.0/16"]
}

resource "azurestack_subnet" "main" {
  name                 = "tf-test-subnet"
  resource_group_name  = azurestack_resource_group.main.name
  virtual_network_name = azurestack_virtual_network.main.name
  address_prefix       = "10.0.1.0/24"
}

# ---------- Network Security Group + rule ----------

resource "azurestack_network_security_group" "main" {
  name                = "tf-test-nsg"
  resource_group_name = azurestack_resource_group.main.name
  location            = azurestack_resource_group.main.location
}

resource "azurestack_network_security_rule" "allow_ssh" {
  name                        = "allow-ssh"
  resource_group_name         = azurestack_resource_group.main.name
  network_security_group_name = azurestack_network_security_group.main.name

  priority                   = 100
  direction                  = "Inbound"
  access                     = "Allow"
  protocol                   = "Tcp"
  source_port_range          = "*"
  destination_port_range     = "22"
  source_address_prefix      = "*"
  destination_address_prefix = "*"
}

# ---------- Storage account (Azure Files / runner shared volumes) ----------

resource "azurestack_storage_account" "main" {
  name                     = "tftestsa12345"
  resource_group_name      = azurestack_resource_group.main.name
  location                 = azurestack_resource_group.main.location
  account_tier             = "Standard"
  account_replication_type = "LRS"
  # azurestack provider validates account_kind ∈ {Storage, BlobStorage};
  # StorageV2 is Azure-public-cloud only. The sim accepts both — but the
  # provider rejects StorageV2 at plan time, so use the older "Storage"
  # kind here.
  account_kind = "Storage"
}

# ---------- Key vault (runner credential storage) ----------

resource "azurestack_key_vault" "main" {
  name                = "tf-test-kv"
  resource_group_name = azurestack_resource_group.main.name
  location            = azurestack_resource_group.main.location
  tenant_id           = "11111111-1111-1111-1111-111111111111"

  sku_name = "standard"
}

# ---------- azurerm-driven resources (azurestack provider doesn't expose these) ----------

# Resource group via azurerm — first check that azurerm can reach the sim
# at all. If this doesn't apply, every other azurerm_* resource below
# would also fail.
resource "azurerm_resource_group" "az_rg" {
  provider = azurerm
  name     = "tf-azrm-rg"
  location = "eastus"
}

# Container Registry — ACA + AZF runner backends both pull container
# images from a private ACR. Standard SKU is the cheapest tier that
# azurerm accepts (Basic / Standard / Premium).
resource "azurerm_container_registry" "az_acr" {
  provider            = azurerm
  name                = "tfazrmacr"
  resource_group_name = azurerm_resource_group.az_rg.name
  location            = azurerm_resource_group.az_rg.location
  sku                 = "Standard"
  admin_enabled       = false
}

# User-assigned managed identity — the runner backends bind one of these
# to each pod/container so it can pull from ACR + read from Key Vault.
resource "azurerm_user_assigned_identity" "az_uai" {
  provider            = azurerm
  name                = "tf-azrm-uai"
  resource_group_name = azurerm_resource_group.az_rg.name
  location            = azurerm_resource_group.az_rg.location
}

# Private DNS zone — sockerless's Azure DNS driver creates one of these
# per cluster to resolve `<service>.internal` to cloud-internal IPs.
resource "azurerm_private_dns_zone" "az_pdns" {
  provider            = azurerm
  name                = "tf-azrm.internal"
  resource_group_name = azurerm_resource_group.az_rg.name
}

# Log Analytics workspace — Container App Environment requires one for
# log ingestion. PerGB2018 is the canonical SKU.
resource "azurerm_log_analytics_workspace" "az_law" {
  provider            = azurerm
  name                = "tf-azrm-law"
  resource_group_name = azurerm_resource_group.az_rg.name
  location            = azurerm_resource_group.az_rg.location
  sku                 = "PerGB2018"
  retention_in_days   = 30
}

# Application Insights — observability companion to Container Apps.
resource "azurerm_application_insights" "az_appins" {
  provider            = azurerm
  name                = "tf-azrm-ai"
  resource_group_name = azurerm_resource_group.az_rg.name
  location            = azurerm_resource_group.az_rg.location
  application_type    = "web"
  workspace_id        = azurerm_log_analytics_workspace.az_law.id
}

# Container App Environment — the ACA host plane. Sockerless's ACA
# backend lives in one of these.
resource "azurerm_container_app_environment" "az_cae" {
  provider                   = azurerm
  name                       = "tf-azrm-cae"
  resource_group_name        = azurerm_resource_group.az_rg.name
  location                   = azurerm_resource_group.az_rg.location
  log_analytics_workspace_id = azurerm_log_analytics_workspace.az_law.id
}

# Container App — the ACA workload. Minimal configuration: one container
# with image + 0.25vCPU/0.5Gi memory + ingress disabled.
resource "azurerm_container_app" "az_ca" {
  provider                     = azurerm
  name                         = "tf-azrm-ca"
  container_app_environment_id = azurerm_container_app_environment.az_cae.id
  resource_group_name          = azurerm_resource_group.az_rg.name
  revision_mode                = "Single"

  template {
    container {
      name   = "main"
      image  = "mcr.microsoft.com/azuredocs/aci-helloworld:latest"
      cpu    = 0.25
      memory = "0.5Gi"
    }
  }
}

# Container App Job — the ACA runner-job primitive. Sockerless dispatches
# CI runner jobs as Container App Jobs.
resource "azurerm_container_app_job" "az_caj" {
  provider                     = azurerm
  name                         = "tf-azrm-caj"
  container_app_environment_id = azurerm_container_app_environment.az_cae.id
  resource_group_name          = azurerm_resource_group.az_rg.name
  location                     = azurerm_resource_group.az_rg.location

  replica_timeout_in_seconds = 60
  replica_retry_limit        = 1

  manual_trigger_config {
    parallelism              = 1
    replica_completion_count = 1
  }

  template {
    container {
      name   = "main"
      image  = "mcr.microsoft.com/azuredocs/aci-helloworld:latest"
      cpu    = 0.25
      memory = "0.5Gi"
    }
  }
}

# Service Plan — Function App host.
resource "azurerm_service_plan" "az_sp" {
  provider            = azurerm
  name                = "tf-azrm-sp"
  resource_group_name = azurerm_resource_group.az_rg.name
  location            = azurerm_resource_group.az_rg.location
  os_type             = "Linux"
  sku_name            = "Y1"
}

# Storage account for the Function App (azurerm-managed storage_account).
# Real Function Apps need a storage account for queue triggers + run
# metadata. Reuses the azurestack_storage_account by name? — actually
# azurerm wants the access_key, and we can't cross-provider that, so
# we create a separate azurerm storage account.
resource "azurerm_storage_account" "az_st" {
  provider                 = azurerm
  name                     = "tfazrmst12345"
  resource_group_name      = azurerm_resource_group.az_rg.name
  location                 = azurerm_resource_group.az_rg.location
  account_tier             = "Standard"
  account_replication_type = "LRS"
}

# Linux Function App — AZF runner backend's host primitive.
resource "azurerm_linux_function_app" "az_fa" {
  provider                   = azurerm
  name                       = "tf-azrm-fa"
  resource_group_name        = azurerm_resource_group.az_rg.name
  location                   = azurerm_resource_group.az_rg.location
  service_plan_id            = azurerm_service_plan.az_sp.id
  storage_account_name       = azurerm_storage_account.az_st.name
  storage_account_access_key = azurerm_storage_account.az_st.primary_access_key

  site_config {}
}

# ---------- Outputs (cross-resource invariants) ----------

output "resource_group_id" {
  value = azurestack_resource_group.main.id
}

output "vnet_id" {
  value = azurestack_virtual_network.main.id
}

output "subnet_id" {
  value = azurestack_subnet.main.id
}

output "nsg_id" {
  value = azurestack_network_security_group.main.id
}

output "nsg_rule_id" {
  value = azurestack_network_security_rule.allow_ssh.id
}

output "storage_account_id" {
  value = azurestack_storage_account.main.id
}

output "storage_account_blob_endpoint" {
  value = azurestack_storage_account.main.primary_blob_endpoint
}

output "key_vault_id" {
  value = azurestack_key_vault.main.id
}

output "key_vault_uri" {
  value = azurestack_key_vault.main.vault_uri
}

output "azrm_resource_group_id" {
  value = azurerm_resource_group.az_rg.id
}

output "azrm_acr_id" {
  value = azurerm_container_registry.az_acr.id
}

output "azrm_uai_id" {
  value = azurerm_user_assigned_identity.az_uai.id
}

output "azrm_private_dns_zone_id" {
  value = azurerm_private_dns_zone.az_pdns.id
}

output "azrm_law_id" {
  value = azurerm_log_analytics_workspace.az_law.id
}

output "azrm_appins_id" {
  value = azurerm_application_insights.az_appins.id
}

output "azrm_container_app_env_id" {
  value = azurerm_container_app_environment.az_cae.id
}

output "azrm_container_app_id" {
  value = azurerm_container_app.az_ca.id
}

output "azrm_container_app_job_id" {
  value = azurerm_container_app_job.az_caj.id
}

output "azrm_service_plan_id" {
  value = azurerm_service_plan.az_sp.id
}

output "azrm_storage_account_id" {
  value = azurerm_storage_account.az_st.id
}

output "azrm_function_app_id" {
  value = azurerm_linux_function_app.az_fa.id
}
