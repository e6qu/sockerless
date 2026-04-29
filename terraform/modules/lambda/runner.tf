# Runner Lambda — `actions/runner` wrapped as a Lambda function.
#
# Each invocation = one ephemeral runner that polls GitHub for one
# job, runs it, and exits. Sockerless-backend-**lambda** is baked
# into the image (see `tests/runners/github/dockerfile-lambda/`) so
# the runner's docker calls dispatch each `container:` sub-task as a
# fresh Lambda invocation (image-mode container Lambda created on
# demand, sharing the workspace EFS access point via FileSystemConfig).
# Workflows stay on Lambda primitives end-to-end, per the project rule
# "backend ↔ host primitive must match".
#
# 15-minute hard cap on Lambda invocations means workflows on this
# runner are restricted to short jobs.
#
# Depends on the ECS-side live env for:
# - EFS filesystem + access points (workspace + externals — same
#   shared access points the runner-task uses, so a Lambda runner
#   and an ECS runner can share state when run sequentially).
# - VPC subnets + security group (for Lambda VPC config so the
#   Lambda can mount EFS).
#
# Looked up via data sources by tag/name so this module doesn't need
# explicit inputs from the ECS module.

data "terraform_remote_state" "ecs" {
  backend = "s3"
  config = {
    bucket = "sockerless-tf-state"
    key    = "environments/ecs/live/terraform.tfstate"
    region = "eu-west-1"
  }
}

locals {
  ecs_state                    = data.terraform_remote_state.ecs.outputs
  ecs_efs_filesystem_id        = local.ecs_state.efs_filesystem_id
  ecs_runner_workspace_apid    = local.ecs_state.runner_workspace_access_point_id
  ecs_private_subnet_ids       = local.ecs_state.private_subnet_ids
  ecs_task_security_group_id   = local.ecs_state.task_security_group_id
  ecs_task_role_arn            = local.ecs_state.task_role_arn
  ecs_execution_role_arn       = local.ecs_state.execution_role_arn
  ecs_log_group_name           = local.ecs_state.log_group_name
}

# Resolve the workspace access point ARN from its ID via a data lookup
# (Lambda's file_system_config requires the full ARN).
data "aws_efs_access_point" "runner_workspace" {
  access_point_id = local.ecs_runner_workspace_apid
}

# IAM role for the runner-Lambda. Broader than a regular Lambda role
# because sockerless-backend-lambda inside the Lambda is itself a
# docker daemon dispatching sub-task Lambdas (one per `container:`
# directive). It needs CreateFunction / Invoke / Delete (image-mode
# container Lambdas), iam:PassRole (to assign this same role to
# the spawned sub-task functions), ECR pull (so spawned Lambdas can
# pull the user image), EFS access (so spawned Lambdas can mount
# the shared workspace access point), and EC2 describe (Lambda VPC
# config validates against the VPC's subnets/SGs).
resource "aws_iam_role" "runner_lambda" {
  name               = "${local.name_prefix}-runner-lambda-role"
  assume_role_policy = data.aws_iam_policy_document.lambda_assume_role.json

  tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-runner-lambda-role"
  })
}

resource "aws_iam_role_policy_attachment" "runner_lambda_basic" {
  role       = aws_iam_role.runner_lambda.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
}

resource "aws_iam_role_policy_attachment" "runner_lambda_vpc" {
  role       = aws_iam_role.runner_lambda.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaVPCAccessExecutionRole"
}

data "aws_iam_policy_document" "runner_lambda" {
  statement {
    sid    = "SockerlessLambdaSubTaskDispatch"
    effect = "Allow"
    actions = [
      "lambda:CreateFunction",
      "lambda:DeleteFunction",
      "lambda:InvokeFunction",
      "lambda:GetFunction",
      "lambda:GetFunctionConfiguration",
      "lambda:UpdateFunctionConfiguration",
      "lambda:UpdateFunctionCode",
      "lambda:ListFunctions",
      "lambda:TagResource",
      "lambda:UntagResource",
      "lambda:ListTags",
    ]
    resources = ["*"]
  }

  # Sockerless's image-inject path inside the runner-Lambda needs to
  # build per-sub-task images via CodeBuild (no docker daemon
  # available inside the Lambda runtime). It uploads the build
  # context to the S3 bucket and starts a CodeBuild project that
  # does the actual `docker build` + `docker push`. See
  # codebuild.tf in this module.
  statement {
    sid    = "CodeBuildImageBuilds"
    effect = "Allow"
    actions = [
      "codebuild:StartBuild",
      "codebuild:BatchGetBuilds",
      "codebuild:StopBuild",
    ]
    resources = [aws_codebuild_project.image_builder.arn]
  }

  statement {
    sid    = "S3BuildContextWrite"
    effect = "Allow"
    actions = [
      "s3:PutObject",
      "s3:GetObject",
      "s3:DeleteObject",
      "s3:ListBucket",
    ]
    resources = [
      aws_s3_bucket.build_context.arn,
      "${aws_s3_bucket.build_context.arn}/*",
    ]
  }

  statement {
    sid    = "PassRoleForSubTasks"
    effect = "Allow"
    actions = ["iam:PassRole"]
    resources = [
      "*",
    ]
  }

  statement {
    sid    = "EC2NetworkDescribe"
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

  statement {
    sid    = "ECRPull"
    effect = "Allow"
    actions = [
      "ecr:GetAuthorizationToken",
      "ecr:BatchCheckLayerAvailability",
      "ecr:GetDownloadUrlForLayer",
      "ecr:BatchGetImage",
      "ecr:DescribeRepositories",
      "ecr:CreatePullThroughCacheRule",
      "ecr:DescribePullThroughCacheRules",
      "ecr:DescribeImages",
    ]
    resources = ["*"]
  }

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
    ]
    resources = ["*"]
  }

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

resource "aws_iam_role_policy" "runner_lambda" {
  name   = "${local.name_prefix}-runner-lambda-policy"
  role   = aws_iam_role.runner_lambda.id
  policy = data.aws_iam_policy_document.runner_lambda.json
}

resource "aws_lambda_function" "sockerless_runner" {
  function_name = "${local.name_prefix}-runner"
  role          = aws_iam_role.runner_lambda.arn
  package_type  = "Image"
  image_uri     = "${aws_ecr_repository.main.repository_url}:runner-amd64"
  architectures = ["x86_64"]
  memory_size   = 3008
  timeout       = 900 # Lambda hard cap
  publish       = true

  # Lambda's default /tmp is 512 MB — too small for the runner state
  # copy (actions/runner externals alone are ~600 MB with node20 +
  # node24 + various tooling). Lambda supports up to 10 GB ephemeral
  # storage; 5 GB is enough for the full runner tree + some
  # workspace scratch space.
  ephemeral_storage {
    size = 5120
  }

  vpc_config {
    subnet_ids         = local.ecs_private_subnet_ids
    security_group_ids = [local.ecs_task_security_group_id]
  }

  file_system_config {
    arn              = data.aws_efs_access_point.runner_workspace.arn
    # Lambda requires file system mount paths under /mnt/. The
    # bootstrap symlinks /home/runner/_work → /mnt/runner-workspace
    # so the runner's hardcoded workspace paths still resolve to
    # the EFS mount.
    local_mount_path = "/mnt/runner-workspace"
  }

  # Lambda only allows ONE file_system_config per function — externals
  # is mounted as a sub-path of the same access point in this Lambda
  # variant. The runner-task on ECS uses two access points; for the
  # Lambda variant we trade off and use a single workspace AP, with
  # the bootstrap pre-populating /home/runner/externals from the
  # image-staged copy (Lambda writes externals into /tmp instead of
  # EFS, since /tmp is writable and large enough for node20).
  #
  # NOTE: Lambda's `file_system_config` block is singular by design.
  # If you need both access points mounted simultaneously, the right
  # path is to pre-populate externals at image-build time inside
  # /home/runner/externals (in the read-only image layer) — the
  # bootstrap copies to /tmp/runner-externals on first invocation.

  environment {
    variables = {
      # AWS_REGION is reserved on Lambda — set automatically by the
      # runtime to the function's region. Don't set explicitly.
      #
      # Sockerless-backend-lambda runs inside this Lambda (project
      # rule: backend ↔ host primitive must match). It dispatches
      # each `container:` sub-task as a fresh Lambda function on
      # demand using these knobs:
      SOCKERLESS_LAMBDA_ROLE_ARN          = aws_iam_role.runner_lambda.arn
      SOCKERLESS_LAMBDA_LOG_GROUP         = "/sockerless/lambda/${local.name_prefix}"
      SOCKERLESS_LAMBDA_SUBNETS           = join(",", local.ecs_private_subnet_ids)
      SOCKERLESS_LAMBDA_SECURITY_GROUPS   = local.ecs_task_security_group_id
      SOCKERLESS_LAMBDA_AGENT_EFS_ID      = local.ecs_efs_filesystem_id
      SOCKERLESS_LAMBDA_ARCHITECTURE      = "x86_64"
      # Bind-mount → EFS translation for sub-task Lambdas. The runner
      # bootstrap stages actions/runner to /tmp/runner-state (Lambda's
      # image filesystem is read-only outside /tmp + EFS), so the
      # runner's bind-mount sources are /tmp/runner-state/_work +
      # /tmp/runner-state/externals etc. Both paths map to the same
      # workspace access point root (Lambda's single-FileSystemConfig
      # constraint); sub-paths under _work (`_temp`, `_actions`, `_tool`)
      # are dropped automatically by sockerless.
      SOCKERLESS_LAMBDA_SHARED_VOLUMES = join(",", [
        "workspace=/tmp/runner-state/_work=${local.ecs_runner_workspace_apid}",
        "externals=/tmp/runner-state/externals=${local.ecs_runner_workspace_apid}",
      ])
      # Image builds for sub-task Lambdas — sockerless-backend-lambda
      # has no docker daemon inside the Lambda runtime, so it
      # delegates `docker build` to AWS CodeBuild via these knobs.
      # See backends/aws-common/build.go::CodeBuildService.
      SOCKERLESS_CODEBUILD_PROJECT = aws_codebuild_project.image_builder.name
      SOCKERLESS_BUILD_BUCKET      = aws_s3_bucket.build_context.bucket
    }
  }

  tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-runner"
  })

  depends_on = [
    aws_iam_role_policy_attachment.runner_lambda_vpc,
    aws_iam_role_policy.runner_lambda,
  ]
}

output "runner_lambda_function_name" {
  value = aws_lambda_function.sockerless_runner.function_name
}

output "runner_lambda_function_arn" {
  value = aws_lambda_function.sockerless_runner.arn
}

output "runner_lambda_role_arn" {
  value = aws_iam_role.runner_lambda.arn
}
