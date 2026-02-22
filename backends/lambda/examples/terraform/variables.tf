variable "project_name" {
  type    = string
  default = "sockerless"
}

variable "environment" {
  type    = string
  default = "example"
}

variable "region" {
  type    = string
  default = "us-east-1"
}

variable "lambda_memory_size" {
  type    = number
  default = 1024
}

variable "lambda_timeout" {
  type    = number
  default = 900
}

variable "log_retention_days" {
  type    = number
  default = 7
}
