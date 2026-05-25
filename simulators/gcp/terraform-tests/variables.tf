variable "endpoint" {
  description = "Simulator endpoint URL"
  type        = string
}

variable "secret_label_env" {
  description = "Secret Manager label value used to exercise UpdateSecret."
  type        = string
  default     = "dev"
}
