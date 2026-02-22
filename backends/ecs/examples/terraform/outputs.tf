output "ecs_cluster_name" {
  value = module.sockerless_ecs.ecs_cluster_name
}

output "private_subnet_ids" {
  value = join(",", module.sockerless_ecs.private_subnet_ids)
}

output "task_security_group_id" {
  value = module.sockerless_ecs.task_security_group_id
}

output "execution_role_arn" {
  value = module.sockerless_ecs.execution_role_arn
}

output "task_role_arn" {
  value = module.sockerless_ecs.task_role_arn
}

output "log_group_name" {
  value = module.sockerless_ecs.log_group_name
}

output "ecr_repository_url" {
  value = module.sockerless_ecs.ecr_repository_url
}

# Print the env vars needed to run the backend
output "backend_env" {
  value = <<-EOT
    export AWS_REGION=${var.region}
    export SOCKERLESS_ECS_CLUSTER=${module.sockerless_ecs.ecs_cluster_name}
    export SOCKERLESS_ECS_SUBNETS=${join(",", module.sockerless_ecs.private_subnet_ids)}
    export SOCKERLESS_ECS_SECURITY_GROUPS=${module.sockerless_ecs.task_security_group_id}
    export SOCKERLESS_ECS_EXECUTION_ROLE_ARN=${module.sockerless_ecs.execution_role_arn}
    export SOCKERLESS_ECS_TASK_ROLE_ARN=${module.sockerless_ecs.task_role_arn}
    export SOCKERLESS_ECS_LOG_GROUP=${module.sockerless_ecs.log_group_name}
  EOT
}
