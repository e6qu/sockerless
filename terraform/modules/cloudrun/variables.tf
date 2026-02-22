variable "project_id" {
  description = "GCP project ID where resources will be created"
  type        = string

  validation {
    condition     = length(var.project_id) > 0
    error_message = "Project ID must not be empty."
  }
}

variable "region" {
  description = "GCP region for all resources"
  type        = string
  default     = "us-central1"
}

variable "project_name" {
  description = "Project name used in resource naming and labels"
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

variable "vpc_connector_machine_type" {
  description = "Machine type for the Serverless VPC Access connector instances"
  type        = string
  default     = "e2-micro"
}

variable "vpc_connector_min_instances" {
  description = "Minimum number of instances for the VPC Access connector"
  type        = number
  default     = 2

  validation {
    condition     = var.vpc_connector_min_instances >= 2
    error_message = "VPC connector minimum instances must be at least 2."
  }
}

variable "vpc_connector_max_instances" {
  description = "Maximum number of instances for the VPC Access connector"
  type        = number
  default     = 3

  validation {
    condition     = var.vpc_connector_max_instances >= 2
    error_message = "VPC connector maximum instances must be at least 2."
  }
}

variable "dns_suffix" {
  description = "DNS suffix for the Cloud DNS private managed zone"
  type        = string
  default     = "sockerless.internal"
}

variable "gcs_location" {
  description = "Location for the GCS bucket (e.g., US, EU, ASIA, or a specific region)"
  type        = string
  default     = "US"
}

variable "gcs_lifecycle_days" {
  description = "Number of days after which GCS objects are deleted by lifecycle rule"
  type        = number
  default     = 30

  validation {
    condition     = var.gcs_lifecycle_days >= 1
    error_message = "GCS lifecycle days must be at least 1."
  }
}

variable "labels" {
  description = "Additional labels to apply to all resources"
  type        = map(string)
  default     = {}
}
