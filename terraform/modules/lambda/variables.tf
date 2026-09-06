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

variable "log_retention_days" {
  description = "Number of days to retain CloudWatch Logs"
  type        = number
  default     = 7

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

variable "lambda_memory_size" {
  description = "Memory allocated to the Lambda function in MB"
  type        = number
  default     = 512

  validation {
    condition     = var.lambda_memory_size >= 128 && var.lambda_memory_size <= 10240
    error_message = "Lambda memory size must be between 128 and 10240 MB."
  }
}

variable "lambda_timeout" {
  description = "Maximum execution time for the Lambda function in seconds (max 900 = 15 minutes)"
  type        = number
  default     = 900

  validation {
    condition     = var.lambda_timeout >= 1 && var.lambda_timeout <= 900
    error_message = "Lambda timeout must be between 1 and 900 seconds (15 minutes max)."
  }
}

variable "tags" {
  description = "Additional tags to apply to all resources"
  type        = map(string)
  default     = {}
}

variable "manage_docker_hub_pull_through_cache" {
  description = "Whether this module owns the docker-hub ECR pull-through cache rule. Pull-through cache rules are singleton per (account, region, prefix); set to false on the lambda module when the ecs module in the same account+region already manages it."
  type        = bool
  default     = true
}

variable "ecs_state_bucket" {
  description = "S3 bucket holding the ECS environment's Terraform state, which this module reads for the EFS, subnet, security-group, role and log-group coordinates the runner Lambda shares with ECS."
  type        = string
  default     = "sockerless-tf-state"
}

variable "ecs_state_key" {
  description = "Key of the ECS environment's Terraform state object in ecs_state_bucket."
  type        = string
  default     = "environments/ecs/live/terraform.tfstate"
}

variable "ecs_state_region" {
  description = "Region of ecs_state_bucket."
  type        = string
  default     = "eu-west-1"
}

variable "read_ecs_state" {
  description = "Whether to read the ECS environment's state for the coordinates the runner Lambda shares with ECS (EFS, subnets, security group, roles, log group). False stands the environment up alone, with none of the shared resources."
  type        = bool
  default     = true
}

variable "ecs_state_local_path" {
  description = "Path of the ECS environment's Terraform state file when that environment keeps its state locally (a simulator environment); read instead of the S3 state when set."
  type        = string
  default     = ""
}
