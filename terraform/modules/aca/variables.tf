variable "location" {
  description = "Azure region for all resources"
  type        = string
  default     = "eastus"
}

variable "project_name" {
  description = "Project name used in resource naming and tags"
  type        = string
  default     = "sockerless"
}

variable "environment" {
  description = "Environment name (e.g., test, staging, production)"
  type        = string

  validation {
    condition     = length(var.environment) > 0
    error_message = "Environment name must not be empty."
  }
}

variable "name_prefix" {
  description = "Prefix used for resource naming to avoid collisions"
  type        = string
  default     = "sockerless"

  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{1,20}$", var.name_prefix))
    error_message = "name_prefix must start with a letter, contain only lowercase alphanumeric characters and hyphens, and be 2-21 characters long."
  }
}

variable "create_resource_group" {
  description = "Whether to create a new resource group or use an existing one"
  type        = bool
  default     = true
}

variable "resource_group_name" {
  description = "Name of an existing resource group to use when create_resource_group is false"
  type        = string
  default     = ""
}

variable "vnet_address_space" {
  description = "Address space for the VNet"
  type        = list(string)
  default     = ["10.0.0.0/16"]

  validation {
    condition     = length(var.vnet_address_space) >= 1
    error_message = "At least one address space must be specified."
  }
}

variable "subnet_address_prefix" {
  description = "Address prefix for the Container Apps subnet. Minimum /23 is required for Container Apps Environment."
  type        = string
  default     = "10.0.0.0/23"

  validation {
    condition     = can(cidrhost(var.subnet_address_prefix, 0))
    error_message = "subnet_address_prefix must be a valid CIDR block."
  }
}

variable "log_retention_days" {
  description = "Number of days to retain logs in the Log Analytics workspace"
  type        = number
  default     = 30

  validation {
    condition     = var.log_retention_days >= 7 && var.log_retention_days <= 730
    error_message = "Log retention must be between 7 and 730 days."
  }
}

variable "storage_account_replication_type" {
  description = "Replication type for the storage account (LRS, GRS, RAGRS, ZRS)"
  type        = string
  default     = "LRS"

  validation {
    condition     = contains(["LRS", "GRS", "RAGRS", "ZRS"], var.storage_account_replication_type)
    error_message = "storage_account_replication_type must be one of: LRS, GRS, RAGRS, ZRS."
  }
}

variable "acr_sku" {
  description = "SKU for Azure Container Registry (Basic, Standard, Premium)"
  type        = string
  default     = "Basic"

  validation {
    condition     = contains(["Basic", "Standard", "Premium"], var.acr_sku)
    error_message = "acr_sku must be one of: Basic, Standard, Premium."
  }
}

variable "file_share_quota" {
  description = "Quota for the Azure Files share in GiB"
  type        = number
  default     = 10

  validation {
    condition     = var.file_share_quota >= 1
    error_message = "File share quota must be at least 1 GiB."
  }
}

variable "tags" {
  description = "Additional tags to apply to all resources"
  type        = map(string)
  default     = {}
}
