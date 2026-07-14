variable "name" {
  description = "Stable name for the Bleephub service and its AWS resources."
  type        = string

  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{1,30}$", var.name))
    error_message = "name must start with a lowercase letter and contain only lowercase letters, digits, and hyphens."
  }
}

variable "region" {
  description = "AWS region for the Bleephub task, buckets, and supporting infrastructure."
  type        = string
  default     = "eu-west-1"
}

variable "vpc_cidr" {
  description = "Dedicated IPv4 CIDR block for the Bleephub VPC. It is never shared with another stack."
  type        = string
  default     = "10.86.0.0/16"
}

variable "availability_zones" {
  description = "At least two Availability Zones in region used for Bleephub public, private, and EFS subnets."
  type        = list(string)
  default     = ["eu-west-1a", "eu-west-1b"]

  validation {
    condition     = length(var.availability_zones) >= 2
    error_message = "Bleephub requires at least two Availability Zones for the Application Load Balancer and EFS."
  }
}

variable "container_image" {
  description = "Immutable Bleephub release image URI, including its registry and digest or immutable tag."
  type        = string
}

variable "hosted_zone_id" {
  description = "Route 53 public hosted-zone ID delegated for the Bleephub hostname."
  type        = string
}

variable "domain_name" {
  description = "Canonical Bleephub DNS name within hosted_zone_id, for example bleephub.e6qu.dev."
  type        = string

  validation {
    condition     = can(regex("^[a-z0-9][a-z0-9.-]+[a-z0-9]$", var.domain_name))
    error_message = "domain_name must be a lowercase DNS name."
  }
}

variable "admin_token" {
  description = "Initial Bleephub administrator token. Terraform stores it only in AWS Secrets Manager and never exposes it as an output."
  type        = string
  sensitive   = true
}

variable "wake_listener_zip_path" {
  description = "Path to the pre-built Linux Amazon Lambda bootstrap ZIP for the Bleephub wake listener. Build it with scripts/build-bleephub-wake.sh before apply."
  type        = string
}

variable "startup_page_path" {
  description = "Path to the extracted startup index.html from the versioned Bleephub startup bundle. Build it with scripts/build-bleephub-startup.sh before apply."
  type        = string
}

variable "github_oauth_client_id" {
  description = "GitHub OAuth App client ID used for Bleephub browser sign-in."
  type        = string
  default     = ""
}

variable "github_oauth_client_secret_arn" {
  description = "Existing AWS Secrets Manager ARN holding the rotated GitHub OAuth App client secret. Terraform never reads or stores its value."
  type        = string
  default     = ""
}

variable "idle_shutdown_minutes" {
  description = "Number of inactive minutes before the ECS service is scaled to zero."
  type        = number
  default     = 5

  validation {
    condition     = var.idle_shutdown_minutes >= 5
    error_message = "idle_shutdown_minutes must be at least five minutes."
  }
}

variable "task_cpu" {
  description = "Fargate CPU units for the single Bleephub task."
  type        = number
  default     = 1024
}

variable "task_memory" {
  description = "Fargate memory MiB for the single Bleephub task."
  type        = number
  default     = 2048
}

variable "log_retention_days" {
  description = "CloudWatch Logs retention for Bleephub application logs."
  type        = number
  default     = 30
}

variable "tags" {
  description = "Additional tags applied to every supported resource."
  type        = map(string)
  default     = {}
}
