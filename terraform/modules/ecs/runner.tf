# Sockerless-runner workload — single-container ECS task definition for
# the GitHub Actions runner with sockerless-backend-ecs baked in. Used
# by the Phase 110b runner harness to dispatch ephemeral runners into
# Fargate. Each task: 1 container that runs sockerless on localhost
# in the background then registers `actions/runner` with `--ephemeral`,
# picks up exactly one job, then exits.
#
# Why single-container vs sidecar pattern: simpler image management
# (one Dockerfile, one push), same architectural correctness (sockerless
# and runner share localhost + EFS workspace mount inside the same
# container). Refactor to sidecar later if image-isolation is desired.

# EFS access point that's shared between the runner-task (mounts at
# /home/runner/_work) and the spawned sub-task (mounts at the runner-
# specified path). Sockerless-backend-ecs (running inside the runner-
# task) sees the runner's `docker create -v /home/runner/_work:/__w`
# bind mount, looks up the matching SharedVolume, and rewrites it to
# a named-volume reference whose access point ID is this resource's
# id. The sub-task task definition then mounts the same access point.
resource "aws_efs_access_point" "runner_workspace" {
  file_system_id = aws_efs_file_system.main.id

  posix_user {
    uid = 1000
    gid = 1000
  }

  root_directory {
    path = "/sockerless-runner-workspace"

    creation_info {
      owner_uid   = 1000
      owner_gid   = 1000
      permissions = "0755"
    }
  }

  tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-runner-workspace"
  })
}

# Second EFS access point for `/home/runner/externals` — the
# JS-action runtime (node20 binary, etc.) that actions/runner
# bind-mounts into job containers. Separate access point with its
# own root path so it's data-isolated from `_work`. The runner-task
# entrypoint pre-populates this from a staged image-baked copy on
# first start; after that, sockerless's sub-task spawns mount the
# same access point at `/__e` so the job container sees node etc.
resource "aws_efs_access_point" "runner_externals" {
  file_system_id = aws_efs_file_system.main.id

  posix_user {
    uid = 1000
    gid = 1000
  }

  root_directory {
    path = "/sockerless-runner-externals"

    creation_info {
      owner_uid   = 1000
      owner_gid   = 1000
      permissions = "0755"
    }
  }

  tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-runner-externals"
  })
}

# IAM role for the runner-task. Strictly broader than the regular
# task role because sockerless-backend-ecs (running inside the task)
# is itself a docker daemon — it needs to RunTask, DescribeTasks,
# RegisterTaskDefinition, manage EFS access points, manage networks
# (security groups), and pull from ECR.
resource "aws_iam_role" "runner_task" {
  name               = "${local.name_prefix}-runner-task-role"
  assume_role_policy = data.aws_iam_policy_document.ecs_assume_role.json

  tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-runner-task-role"
  })
}

data "aws_iam_policy_document" "runner_task" {
  # Sockerless-backend-ecs dispatches sub-tasks. Needs full ECS task
  # lifecycle + task-definition management + ExecuteCommand for the
  # `docker exec` flow that GitHub Actions' runner uses to run each
  # workflow step inside the spawned job container.
  statement {
    sid    = "SockerlessECSDispatch"
    effect = "Allow"
    actions = [
      "ecs:RunTask",
      "ecs:StopTask",
      "ecs:DescribeTasks",
      "ecs:ListTasks",
      "ecs:RegisterTaskDefinition",
      "ecs:DeregisterTaskDefinition",
      "ecs:DescribeTaskDefinition",
      "ecs:ListTaskDefinitions",
      "ecs:TagResource",
      "ecs:UntagResource",
      "ecs:DescribeServices",
      "ecs:DescribeContainerInstances",
      "ecs:DescribeClusters",
      "ecs:ExecuteCommand",
    ]
    resources = ["*"]
  }

  # Pass the existing execution + task roles to sub-tasks. Without
  # this, RegisterTaskDefinition with roleArn / executionRoleArn
  # rejects the request.
  statement {
    sid    = "PassRoleForSubTasks"
    effect = "Allow"
    actions = ["iam:PassRole"]
    resources = [
      aws_iam_role.execution.arn,
      aws_iam_role.task.arn,
      aws_iam_role.runner_task.arn,
    ]
  }

  # Network discovery for sub-task RunTask (subnets, SGs, ENIs).
  statement {
    sid    = "EC2Network"
    effect = "Allow"
    actions = [
      "ec2:DescribeSubnets",
      "ec2:DescribeSecurityGroups",
      "ec2:DescribeNetworkInterfaces",
      "ec2:DescribeVpcs",
      "ec2:DescribeAvailabilityZones",
      "ec2:CreateSecurityGroup",
      "ec2:DeleteSecurityGroup",
      "ec2:AuthorizeSecurityGroupIngress",
      "ec2:RevokeSecurityGroupIngress",
      "ec2:CreateTags",
    ]
    resources = ["*"]
  }

  # ECR for image pulls + pull-through cache (alpine etc. via
  # public.ecr.aws routing).
  statement {
    sid    = "ECRPull"
    effect = "Allow"
    actions = [
      "ecr:GetAuthorizationToken",
      "ecr:BatchCheckLayerAvailability",
      "ecr:GetDownloadUrlForLayer",
      "ecr:BatchGetImage",
      "ecr:DescribeRepositories",
      "ecr:CreateRepository",
      "ecr:CreatePullThroughCacheRule",
      "ecr:DescribePullThroughCacheRules",
      "ecr:DescribeImages",
      "ecr:ListImages",
    ]
    resources = ["*"]
  }

  # EFS access-point management for sub-task workspace volumes.
  statement {
    sid    = "EFSAccess"
    effect = "Allow"
    actions = [
      "elasticfilesystem:ClientMount",
      "elasticfilesystem:ClientWrite",
      "elasticfilesystem:ClientRootAccess",
      "elasticfilesystem:DescribeAccessPoints",
      "elasticfilesystem:DescribeFileSystems",
      "elasticfilesystem:DescribeMountTargets",
      "elasticfilesystem:CreateAccessPoint",
      "elasticfilesystem:DeleteAccessPoint",
      "elasticfilesystem:TagResource",
      "elasticfilesystem:UntagResource",
    ]
    resources = ["*"]
  }

  # Cloud Map service discovery (sub-tasks register hostnames).
  statement {
    sid    = "CloudMap"
    effect = "Allow"
    actions = [
      "servicediscovery:RegisterInstance",
      "servicediscovery:DeregisterInstance",
      "servicediscovery:CreateService",
      "servicediscovery:GetService",
      "servicediscovery:ListServices",
      "servicediscovery:DeleteService",
      "servicediscovery:CreateHttpNamespace",
      "servicediscovery:CreatePrivateDnsNamespace",
      "servicediscovery:DeleteNamespace",
      "servicediscovery:GetNamespace",
      "servicediscovery:ListNamespaces",
      "servicediscovery:GetOperation",
      "servicediscovery:TagResource",
      "servicediscovery:UntagResource",
    ]
    resources = ["*"]
  }

  # CloudWatch Logs (own logs + sub-task logs).
  statement {
    sid    = "CWLogs"
    effect = "Allow"
    actions = [
      "logs:CreateLogStream",
      "logs:PutLogEvents",
      "logs:CreateLogGroup",
      "logs:DescribeLogStreams",
      "logs:GetLogEvents",
    ]
    resources = ["*"]
  }

  # SSM messages for ECS Exec on sub-tasks (BUG-720 / 842).
  statement {
    sid    = "SSMMessages"
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

resource "aws_iam_role_policy" "runner_task" {
  name   = "${local.name_prefix}-runner-task-policy"
  role   = aws_iam_role.runner_task.id
  policy = data.aws_iam_policy_document.runner_task.json
}

# Single-container task definition. Operator overrides per-task env
# vars (RUNNER_TOKEN, RUNNER_NAME, RUNNER_LABELS, RUNNER_REPO_URL) at
# RunTask time via container overrides — keeps the registered
# definition reusable across runs.
resource "aws_ecs_task_definition" "sockerless_runner" {
  family                   = "${local.name_prefix}-runner"
  network_mode             = "awsvpc"
  requires_compatibilities = ["FARGATE"]
  cpu                      = "1024"
  memory                   = "2048"
  task_role_arn            = aws_iam_role.runner_task.arn
  execution_role_arn       = aws_iam_role.execution.arn

  runtime_platform {
    operating_system_family = "LINUX"
    cpu_architecture        = "X86_64"
  }

  volume {
    name = "workspace"

    efs_volume_configuration {
      file_system_id     = aws_efs_file_system.main.id
      transit_encryption = "ENABLED"

      authorization_config {
        access_point_id = aws_efs_access_point.runner_workspace.id
        iam             = "ENABLED"
      }
    }
  }

  volume {
    name = "externals"

    efs_volume_configuration {
      file_system_id     = aws_efs_file_system.main.id
      transit_encryption = "ENABLED"

      authorization_config {
        access_point_id = aws_efs_access_point.runner_externals.id
        iam             = "ENABLED"
      }
    }
  }

  container_definitions = jsonencode([
    {
      name      = "runner"
      image     = "${aws_ecr_repository.main.repository_url}:runner-amd64"
      essential = true

      mountPoints = [
        {
          sourceVolume  = "workspace"
          containerPath = "/home/runner/_work"
          readOnly      = false
        },
        {
          sourceVolume  = "externals"
          containerPath = "/home/runner/externals"
          readOnly      = false
        }
      ]

      environment = [
        # AWS context for sockerless-backend-ecs.
        { name = "AWS_REGION", value = var.region },
        { name = "SOCKERLESS_ECS_CLUSTER", value = aws_ecs_cluster.main.name },
        { name = "SOCKERLESS_ECS_SUBNETS", value = join(",", aws_subnet.private[*].id) },
        { name = "SOCKERLESS_ECS_SECURITY_GROUPS", value = aws_security_group.task[0].id },
        { name = "SOCKERLESS_ECS_TASK_ROLE_ARN", value = aws_iam_role.task.arn },
        { name = "SOCKERLESS_ECS_EXECUTION_ROLE_ARN", value = aws_iam_role.execution.arn },
        { name = "SOCKERLESS_ECS_LOG_GROUP", value = aws_cloudwatch_log_group.main.name },
        { name = "SOCKERLESS_AGENT_EFS_ID", value = aws_efs_file_system.main.id },
        { name = "SOCKERLESS_ECS_PUBLIC_IP", value = "false" },
        { name = "SOCKERLESS_ECS_CPU_ARCHITECTURE", value = "X86_64" },
        # Bind-mount → EFS translation: when runner does
        # `docker create -v /home/runner/_work:/__w alpine`, sockerless
        # rewrites to `-v workspace:/__w` and the spawned sub-task
        # mounts the same EFS access point.
        {
          name  = "SOCKERLESS_ECS_SHARED_VOLUMES"
          value = "workspace=/home/runner/_work=${aws_efs_access_point.runner_workspace.id},externals=/home/runner/externals=${aws_efs_access_point.runner_externals.id}"
        },
      ]

      logConfiguration = {
        logDriver = "awslogs"
        options = {
          awslogs-group         = aws_cloudwatch_log_group.main.name
          awslogs-region        = var.region
          awslogs-stream-prefix = "runner"
        }
      }
    }
  ])

  tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-runner"
  })
}

output "runner_task_definition_arn" {
  value = aws_ecs_task_definition.sockerless_runner.arn
}

output "runner_task_definition_family" {
  value = aws_ecs_task_definition.sockerless_runner.family
}

output "runner_workspace_access_point_id" {
  value = aws_efs_access_point.runner_workspace.id
}

output "runner_externals_access_point_id" {
  value = aws_efs_access_point.runner_externals.id
}

output "runner_task_role_arn" {
  value = aws_iam_role.runner_task.arn
}
