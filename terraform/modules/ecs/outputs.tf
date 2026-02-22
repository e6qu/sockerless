# =============================================================================
# VPC and Networking
# =============================================================================

output "vpc_id" {
  description = "ID of the VPC"
  value       = aws_vpc.main.id
}

output "private_subnet_ids" {
  description = "List of private subnet IDs where Fargate tasks run"
  value       = aws_subnet.private[*].id
}

output "public_subnet_ids" {
  description = "List of public subnet IDs (NAT Gateway placement)"
  value       = aws_subnet.public[*].id
}

# =============================================================================
# ECS Cluster
# =============================================================================

output "ecs_cluster_arn" {
  description = "ARN of the ECS Fargate cluster"
  value       = aws_ecs_cluster.main.arn
}

output "ecs_cluster_name" {
  description = "Name of the ECS Fargate cluster"
  value       = aws_ecs_cluster.main.name
}

# =============================================================================
# EFS
# =============================================================================

output "efs_filesystem_id" {
  description = "ID of the EFS filesystem for shared volumes"
  value       = aws_efs_file_system.main.id
}

# =============================================================================
# CloudWatch Logs
# =============================================================================

output "log_group_name" {
  description = "Name of the CloudWatch Logs log group for container logs"
  value       = aws_cloudwatch_log_group.main.name
}

output "log_group_arn" {
  description = "ARN of the CloudWatch Logs log group"
  value       = aws_cloudwatch_log_group.main.arn
}

# =============================================================================
# Cloud Map
# =============================================================================

output "cloud_map_namespace_id" {
  description = "ID of the Cloud Map private DNS namespace"
  value       = aws_service_discovery_private_dns_namespace.main.id
}

output "cloud_map_namespace_arn" {
  description = "ARN of the Cloud Map private DNS namespace"
  value       = aws_service_discovery_private_dns_namespace.main.arn
}

# =============================================================================
# IAM Roles
# =============================================================================

output "execution_role_arn" {
  description = "ARN of the ECS task execution role (image pull, log write)"
  value       = aws_iam_role.execution.arn
}

output "task_role_arn" {
  description = "ARN of the ECS task role (EFS, CloudWatch, Cloud Map)"
  value       = aws_iam_role.task.arn
}

# =============================================================================
# Security Groups
# =============================================================================

output "task_security_group_id" {
  description = "ID of the task security group for Fargate tasks"
  value       = aws_security_group.task.id
}

output "efs_security_group_id" {
  description = "ID of the EFS mount target security group"
  value       = aws_security_group.efs.id
}

# =============================================================================
# ECR
# =============================================================================

output "ecr_repository_url" {
  description = "URL of the ECR repository for container images"
  value       = aws_ecr_repository.main.repository_url
}
