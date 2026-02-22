# =============================================================================
# IAM
# =============================================================================

output "execution_role_arn" {
  description = "ARN of the Lambda execution role (image pull, log write)"
  value       = aws_iam_role.execution.arn
}

# =============================================================================
# CloudWatch Logs
# =============================================================================

output "log_group_name" {
  description = "Name of the CloudWatch Logs log group for Lambda function logs"
  value       = aws_cloudwatch_log_group.main.name
}

output "log_group_arn" {
  description = "ARN of the CloudWatch Logs log group"
  value       = aws_cloudwatch_log_group.main.arn
}

# =============================================================================
# ECR
# =============================================================================

output "ecr_repository_url" {
  description = "URL of the ECR repository for Lambda container images"
  value       = aws_ecr_repository.main.repository_url
}

output "ecr_repository_arn" {
  description = "ARN of the ECR repository"
  value       = aws_ecr_repository.main.arn
}
