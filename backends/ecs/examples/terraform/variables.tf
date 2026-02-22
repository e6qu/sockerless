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

variable "vpc_cidr" {
  type    = string
  default = "10.99.0.0/16"
}

variable "availability_zones" {
  type    = list(string)
  default = ["us-east-1a", "us-east-1b"]
}

variable "log_retention_days" {
  type    = number
  default = 1
}
