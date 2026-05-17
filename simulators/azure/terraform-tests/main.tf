terraform {
  required_providers {
    azurestack = {
      source  = "hashicorp/azurestack"
      version = "~> 1.0"
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
