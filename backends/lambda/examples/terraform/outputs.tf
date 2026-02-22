output "execution_role_arn" {
  value = module.sockerless_lambda.execution_role_arn
}

output "log_group_name" {
  value = module.sockerless_lambda.log_group_name
}

output "ecr_repository_url" {
  value = module.sockerless_lambda.ecr_repository_url
}

# Print the env vars needed to run the backend
output "backend_env" {
  value = <<-EOT
    export AWS_REGION=${var.region}
    export SOCKERLESS_LAMBDA_ROLE_ARN=${module.sockerless_lambda.execution_role_arn}
    export SOCKERLESS_LAMBDA_MEMORY_SIZE=${var.lambda_memory_size}
    export SOCKERLESS_LAMBDA_TIMEOUT=${var.lambda_timeout}
    export SOCKERLESS_CALLBACK_URL=http://<YOUR_BACKEND_HOST>:9100
  EOT
}
