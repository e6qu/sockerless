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

variable "region" {
  description = "AWS region for all resources"
  type        = string
  default     = "us-east-1"
}

variable "vpc_cidr" {
  description = "CIDR block for the VPC"
  type        = string
  default     = "10.99.0.0/16"

  validation {
    condition     = can(cidrhost(var.vpc_cidr, 0))
    error_message = "vpc_cidr must be a valid CIDR block."
  }
}

variable "availability_zones" {
  description = "List of availability zones for subnet placement"
  type        = list(string)
  default     = ["us-east-1a", "us-east-1b"]

  validation {
    condition     = length(var.availability_zones) >= 1
    error_message = "At least one availability zone must be specified."
  }
}

variable "nat_gateway_count" {
  description = "Number of NAT Gateways to create. Set to 1 for cost savings or length(availability_zones) for HA."
  type        = number
  default     = 1

  validation {
    condition     = var.nat_gateway_count >= 1
    error_message = "At least one NAT Gateway is required."
  }
}

variable "efs_encrypted" {
  description = "Whether to encrypt the EFS filesystem at rest"
  type        = bool
  default     = true
}

variable "log_retention_days" {
  description = "Number of days to retain CloudWatch Logs"
  type        = number
  default     = 1

  validation {
    condition     = var.log_retention_days >= 1
    error_message = "Log retention must be at least 1 day."
  }
}

variable "ecr_image_expiry_days" {
  description = "Number of days after which untagged ECR images are expired"
  type        = number
  default     = 7

  validation {
    condition     = var.ecr_image_expiry_days >= 1
    error_message = "ECR image expiry must be at least 1 day."
  }
}

variable "tags" {
  description = "Additional tags to apply to all resources"
  type        = map(string)
  default     = {}
}

variable "manage_docker_hub_pull_through_cache" {
  description = "Whether this module owns the docker-hub ECR pull-through cache rule. Pull-through cache rules are singleton per (account, region, prefix); set to false on the ecs module when the lambda module in the same account+region already manages it."
  type        = bool
  default     = true
}

# Optional: use an existing VPC instead of creating one.
# When vpc_id is set, the module skips VPC/subnet/NAT creation and uses the provided values.
variable "existing_vpc_id" {
  description = "ID of an existing VPC to use. If set, subnet_ids and security_group_id must also be set."
  type        = string
  default     = ""
}

variable "existing_subnet_ids" {
  description = "List of existing private subnet IDs to use when existing_vpc_id is set."
  type        = list(string)
  default     = []
}

variable "existing_security_group_id" {
  description = "Existing security group ID for ECS tasks when existing_vpc_id is set."
  type        = string
  default     = ""
}
