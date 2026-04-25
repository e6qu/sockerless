# =============================================================================
# ECS Terraform Module
# =============================================================================
#
# Provisions all AWS infrastructure required by the Sockerless ECS Fargate
# backend. This includes:
#   - VPC with public and private subnets, NAT Gateway, and Internet Gateway
#   - ECS Fargate cluster with Container Insights
#   - EFS filesystem with mount targets in each private subnet
#   - CloudWatch Logs log group
#   - Cloud Map private DNS namespace for service discovery
#   - ECR repository for container images
#   - IAM execution and task roles with least-privilege policies
#   - Security groups for tasks and EFS mount targets
#
# Prerequisites:
#   - AWS provider configured with appropriate credentials
#   - Terraform >= 1.5
#
# Usage:
#   module "ecs" {
#     source      = "../../modules/ecs"
#     environment = "test"
#   }
# =============================================================================

locals {
  name_prefix = "${var.project_name}-${var.environment}"

  # When existing VPC is provided, use it; otherwise use the created VPC
  use_existing_vpc = var.existing_vpc_id != ""
  vpc_id           = local.use_existing_vpc ? var.existing_vpc_id : aws_vpc.main[0].id
  subnet_ids       = local.use_existing_vpc ? var.existing_subnet_ids : aws_subnet.private[*].id
  task_sg_id       = local.use_existing_vpc ? var.existing_security_group_id : aws_security_group.task[0].id

  common_tags = merge(var.tags, {
    project     = var.project_name
    environment = var.environment
    component   = "ecs"
    managed-by  = "terraform"
  })
}

# =============================================================================
# Data Sources
# =============================================================================

data "aws_caller_identity" "current" {}

data "aws_partition" "current" {}

# =============================================================================
# VPC
# =============================================================================

resource "aws_vpc" "main" {
  count = local.use_existing_vpc ? 0 : 1

  cidr_block           = var.vpc_cidr
  enable_dns_support   = true
  enable_dns_hostnames = true

  tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-vpc"
  })
}

# Sweep sockerless-runtime drift on `terragrunt destroy` so the VPC
# isn't held back by Cloud Map namespaces, skls-* security groups, or
# sockerless-owned EFS filesystems that are tagged but not part of
# this module's state (runtime-created by `docker network create`,
# `docker volume create`, etc.). Runs in its own null_resource gated
# on the VPC's existence so terraform orders it before VPC delete.
resource "null_resource" "sockerless_runtime_sweep" {
  count = local.use_existing_vpc ? 0 : 1

  triggers = {
    vpc_id = aws_vpc.main[0].id
    region = var.region
  }

  provisioner "local-exec" {
    when    = destroy
    command = <<-EOT
      set -eu
      region='${self.triggers.region}'
      vpc='${self.triggers.vpc_id}'
      echo "sockerless-runtime-sweep: region=$region vpc=$vpc"

      # Cloud Map: delete sockerless-created namespaces (skls-*).
      for ns_id in $(aws servicediscovery list-namespaces --region "$region" --query 'Namespaces[?starts_with(Name,`skls`)].Id' --output text); do
        [ -z "$ns_id" ] && continue
        for svc_id in $(aws servicediscovery list-services --region "$region" --filters "Name=NAMESPACE_ID,Values=$ns_id" --query 'Services[].Id' --output text); do
          [ -z "$svc_id" ] && continue
          for inst in $(aws servicediscovery list-instances --region "$region" --service-id "$svc_id" --query 'Instances[].Id' --output text); do
            [ -z "$inst" ] && continue
            aws servicediscovery deregister-instance --region "$region" --service-id "$svc_id" --instance-id "$inst" >/dev/null || true
          done
          sleep 2
          aws servicediscovery delete-service --region "$region" --id "$svc_id" >/dev/null || true
        done
        sleep 3
        aws servicediscovery delete-namespace --region "$region" --id "$ns_id" >/dev/null || true
      done

      # Security groups: delete sockerless-created network SGs (skls-*).
      for sg in $(aws ec2 describe-security-groups --region "$region" --filters "Name=vpc-id,Values=$vpc" "Name=group-name,Values=skls-*" --query 'SecurityGroups[].GroupId' --output text); do
        [ -z "$sg" ] && continue
        aws ec2 delete-security-group --region "$region" --group-id "$sg" || true
      done

      # EFS: sockerless runtime sometimes creates its own filesystem
      # (tagged sockerless-managed=true) when the operator hasn't wired
      # SOCKERLESS_AGENT_EFS_ID. Drain mount targets then delete.
      for fs in $(aws efs describe-file-systems --region "$region" --query 'FileSystems[?Tags[?Key==`sockerless-managed`&&Value==`true`]].FileSystemId' --output text); do
        [ -z "$fs" ] && continue
        for mt in $(aws efs describe-mount-targets --region "$region" --file-system-id "$fs" --query 'MountTargets[].MountTargetId' --output text); do
          [ -z "$mt" ] && continue
          aws efs delete-mount-target --region "$region" --mount-target-id "$mt" || true
        done
        # Wait for mount targets to drain (delete-file-system requires it).
        for i in 1 2 3 4 5 6 7 8; do
          left=$(aws efs describe-mount-targets --region "$region" --file-system-id "$fs" --query 'length(MountTargets)' --output text 2>/dev/null || echo 0)
          [ "$left" = "0" ] && break
          sleep 10
        done
        aws efs delete-file-system --region "$region" --file-system-id "$fs" || true
      done
    EOT
  }
}

# =============================================================================
# Internet Gateway
# =============================================================================

resource "aws_internet_gateway" "main" {
  count = local.use_existing_vpc ? 0 : 1

  vpc_id = aws_vpc.main[0].id

  tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-igw"
  })
}

# =============================================================================
# Public Subnets
# =============================================================================

resource "aws_subnet" "public" {
  count = local.use_existing_vpc ? 0 : length(var.availability_zones)

  vpc_id                  = aws_vpc.main[0].id
  cidr_block              = cidrsubnet(var.vpc_cidr, 8, count.index)
  availability_zone       = var.availability_zones[count.index]
  map_public_ip_on_launch = true

  tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-public-${var.availability_zones[count.index]}"
    tier = "public"
  })
}

# =============================================================================
# Private Subnets
# =============================================================================

resource "aws_subnet" "private" {
  count = local.use_existing_vpc ? 0 : length(var.availability_zones)

  vpc_id            = aws_vpc.main[0].id
  cidr_block        = cidrsubnet(var.vpc_cidr, 8, count.index + length(var.availability_zones))
  availability_zone = var.availability_zones[count.index]

  tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-private-${var.availability_zones[count.index]}"
    tier = "private"
  })
}

# =============================================================================
# NAT Gateway(s) with Elastic IPs
# =============================================================================

resource "aws_eip" "nat" {
  count = local.use_existing_vpc ? 0 : var.nat_gateway_count

  domain = "vpc"

  tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-nat-eip-${count.index}"
  })
}

resource "aws_nat_gateway" "main" {
  count = local.use_existing_vpc ? 0 : var.nat_gateway_count

  allocation_id = aws_eip.nat[count.index].id
  subnet_id     = aws_subnet.public[count.index].id

  tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-nat-${count.index}"
  })

  depends_on = [aws_internet_gateway.main]
}

# =============================================================================
# Route Tables
# =============================================================================

# Public route table — routes to Internet Gateway
resource "aws_route_table" "public" {
  count = local.use_existing_vpc ? 0 : 1

  vpc_id = aws_vpc.main[0].id

  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.main[0].id
  }

  tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-public-rt"
  })
}

resource "aws_route_table_association" "public" {
  count = local.use_existing_vpc ? 0 : length(var.availability_zones)

  subnet_id      = aws_subnet.public[count.index].id
  route_table_id = aws_route_table.public[0].id
}

# Private route tables — route to NAT Gateway(s)
resource "aws_route_table" "private" {
  count = local.use_existing_vpc ? 0 : length(var.availability_zones)

  vpc_id = aws_vpc.main[0].id

  route {
    cidr_block     = "0.0.0.0/0"
    nat_gateway_id = aws_nat_gateway.main[count.index % var.nat_gateway_count].id
  }

  tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-private-rt-${var.availability_zones[count.index]}"
  })
}

resource "aws_route_table_association" "private" {
  count = local.use_existing_vpc ? 0 : length(var.availability_zones)

  subnet_id      = aws_subnet.private[count.index].id
  route_table_id = aws_route_table.private[count.index].id
}

# =============================================================================
# ECS Cluster
# =============================================================================

resource "aws_ecs_cluster" "main" {
  name = local.name_prefix

  setting {
    name  = "containerInsights"
    value = "enabled"
  }

  tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-cluster"
  })
}

# =============================================================================
# EFS Filesystem
# =============================================================================

resource "aws_efs_file_system" "main" {
  encrypted        = var.efs_encrypted
  performance_mode = "generalPurpose"
  throughput_mode  = "bursting"

  lifecycle_policy {
    transition_to_ia = "AFTER_30_DAYS"
  }

  tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-efs"
  })
}

resource "aws_efs_mount_target" "main" {
  count = length(local.subnet_ids)

  file_system_id  = aws_efs_file_system.main.id
  subnet_id       = local.subnet_ids[count.index]
  security_groups = [aws_security_group.efs.id]
}

# =============================================================================
# EFS Security Group
# =============================================================================

resource "aws_security_group" "efs" {
  name        = "${local.name_prefix}-efs-sg"
  description = "Security group for EFS mount targets - allows NFS from task security group"
  vpc_id      = local.vpc_id

  tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-efs-sg"
  })
}

resource "aws_security_group_rule" "efs_inbound_nfs" {
  description              = "Allow NFS from task security group"
  type                     = "ingress"
  from_port                = 2049
  to_port                  = 2049
  protocol                 = "tcp"
  source_security_group_id = local.task_sg_id
  security_group_id        = aws_security_group.efs.id
}

# =============================================================================
# CloudWatch Logs
# =============================================================================

resource "aws_cloudwatch_log_group" "main" {
  name              = "/${var.project_name}/${var.environment}/containers"
  retention_in_days = var.log_retention_days

  tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-log-group"
  })
}

# =============================================================================
# Cloud Map (Service Discovery)
# =============================================================================

resource "aws_service_discovery_private_dns_namespace" "main" {
  name        = "${local.name_prefix}.local"
  description = "Private DNS namespace for ${local.name_prefix} service discovery"
  vpc         = local.vpc_id

  tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-namespace"
  })
}

# =============================================================================
# ECR Repository
# =============================================================================

resource "aws_ecr_repository" "main" {
  name                 = local.name_prefix
  image_tag_mutability = "MUTABLE"

  image_scanning_configuration {
    scan_on_push = true
  }

  tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-ecr"
  })
}

resource "aws_ecr_lifecycle_policy" "main" {
  repository = aws_ecr_repository.main.name

  policy = jsonencode({
    rules = [
      {
        rulePriority = 1
        description  = "Expire untagged images after ${var.ecr_image_expiry_days} days"
        selection = {
          tagStatus   = "untagged"
          countType   = "sinceImagePushed"
          countUnit   = "days"
          countNumber = var.ecr_image_expiry_days
        }
        action = {
          type = "expire"
        }
      }
    ]
  })
}

# =============================================================================
# IAM — ECS Task Execution Role
# =============================================================================

data "aws_iam_policy_document" "ecs_assume_role" {
  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["ecs-tasks.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "execution" {
  name               = "${local.name_prefix}-execution-role"
  assume_role_policy = data.aws_iam_policy_document.ecs_assume_role.json

  tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-execution-role"
  })
}

resource "aws_iam_role_policy_attachment" "execution_managed" {
  role       = aws_iam_role.execution.name
  policy_arn = "arn:${data.aws_partition.current.partition}:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

data "aws_iam_policy_document" "execution_ecr" {
  statement {
    sid    = "ECRPull"
    effect = "Allow"
    actions = [
      "ecr:GetAuthorizationToken",
      "ecr:BatchCheckLayerAvailability",
      "ecr:GetDownloadUrlForLayer",
      "ecr:BatchGetImage",
    ]
    resources = ["*"]
  }
}

resource "aws_iam_role_policy" "execution_ecr" {
  name   = "${local.name_prefix}-execution-ecr"
  role   = aws_iam_role.execution.id
  policy = data.aws_iam_policy_document.execution_ecr.json
}

# =============================================================================
# IAM — ECS Task Role
# =============================================================================

resource "aws_iam_role" "task" {
  name               = "${local.name_prefix}-task-role"
  assume_role_policy = data.aws_iam_policy_document.ecs_assume_role.json

  tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-task-role"
  })
}

data "aws_iam_policy_document" "task" {
  # EFS access
  statement {
    sid    = "EFSAccess"
    effect = "Allow"
    actions = [
      "elasticfilesystem:ClientMount",
      "elasticfilesystem:ClientWrite",
      "elasticfilesystem:DescribeMountTargets",
    ]
    resources = [aws_efs_file_system.main.arn]
  }

  # CloudWatch Logs write access
  statement {
    sid    = "CloudWatchLogs"
    effect = "Allow"
    actions = [
      "logs:CreateLogStream",
      "logs:PutLogEvents",
    ]
    resources = ["${aws_cloudwatch_log_group.main.arn}:*"]
  }

  # Cloud Map service discovery
  statement {
    sid    = "CloudMapDiscovery"
    effect = "Allow"
    actions = [
      "servicediscovery:RegisterInstance",
      "servicediscovery:DeregisterInstance",
    ]
    resources = ["*"]
  }

  # SSM messages — required for ECS Exec (BUG-720). Without these,
  # ECS.ExecuteCommand returns an error or the SSM data channel
  # immediately closes when the agent inside the task tries to dial back
  # to SSM Session Manager.
  statement {
    sid    = "ECSExecSSMMessages"
    effect = "Allow"
    actions = [
      "ssmmessages:CreateControlChannel",
      "ssmmessages:CreateDataChannel",
      "ssmmessages:OpenControlChannel",
      "ssmmessages:OpenDataChannel",
    ]
    resources = ["*"]
  }
}

resource "aws_iam_role_policy" "task" {
  name   = "${local.name_prefix}-task-policy"
  role   = aws_iam_role.task.id
  policy = data.aws_iam_policy_document.task.json
}

# =============================================================================
# Task Security Group
# =============================================================================

resource "aws_security_group" "task" {
  count = local.use_existing_vpc ? 0 : 1

  name        = "${local.name_prefix}-task-sg"
  description = "Security group for ECS Fargate tasks"
  vpc_id      = aws_vpc.main[0].id

  tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-task-sg"
  })
}

# Self-referencing rule: all traffic between tasks in this security group
resource "aws_security_group_rule" "task_self_ingress" {
  count = local.use_existing_vpc ? 0 : 1

  description       = "Allow all traffic from tasks in the same security group"
  type              = "ingress"
  from_port         = 0
  to_port           = 65535
  protocol          = "tcp"
  self              = true
  security_group_id = aws_security_group.task[0].id
}

# Agent port: inbound TCP 9111 from self (for agent connectivity)
resource "aws_security_group_rule" "task_agent_ingress" {
  count = local.use_existing_vpc ? 0 : 1

  description       = "Allow agent connectivity on port 9111 from within the security group"
  type              = "ingress"
  from_port         = 9111
  to_port           = 9111
  protocol          = "tcp"
  self              = true
  security_group_id = aws_security_group.task[0].id
}

# All outbound traffic
resource "aws_security_group_rule" "task_all_egress" {
  count = local.use_existing_vpc ? 0 : 1

  description       = "Allow all outbound traffic"
  type              = "egress"
  from_port         = 0
  to_port           = 0
  protocol          = "-1"
  cidr_blocks       = ["0.0.0.0/0"]
  security_group_id = aws_security_group.task[0].id
}
