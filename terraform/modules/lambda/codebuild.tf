# CodeBuild project + S3 build-context bucket. Used by:
#
# 1. Manual runner-Lambda image rebuild (`make codebuild-update`
#    drives this project to rebuild from a tarball uploaded to S3).
# 2. The runner-Lambda's bundled `sockerless-backend-lambda` at
#    runtime: when a workflow's `container:` directive triggers a
#    sub-task Lambda creation, the lambda backend uses
#    `awscommon.NewCodeBuildService` + `SOCKERLESS_CODEBUILD_PROJECT`
#    + `SOCKERLESS_BUILD_BUCKET` to build the per-sub-task image
#    (image-mode container Lambda with sockerless-agent + bootstrap
#    injected). This is what enables Lambda-in-Lambda dispatch
#    without needing a docker daemon inside the runner-Lambda.

resource "aws_s3_bucket" "build_context" {
  bucket = "${local.name_prefix}-build-context"

  tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-build-context"
  })
}

resource "aws_s3_bucket_lifecycle_configuration" "build_context_expire" {
  bucket = aws_s3_bucket.build_context.id

  # Build-context tarballs are single-use; CodeBuild downloads them
  # at the start of each build and never re-reads. 24-hour expiration
  # is plenty of headroom for in-flight builds + leaves no debris.
  rule {
    id     = "expire-build-contexts"
    status = "Enabled"

    filter { prefix = "build-context/" }

    expiration { days = 1 }
  }
}

resource "aws_s3_bucket_public_access_block" "build_context" {
  bucket                  = aws_s3_bucket.build_context.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

# CodeBuild service role. Needs S3 read on the build-context bucket,
# ECR push to the live repo, CloudWatch Logs for build output.
resource "aws_iam_role" "codebuild" {
  name = "${local.name_prefix}-codebuild-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Principal = { Service = "codebuild.amazonaws.com" }
      Action = "sts:AssumeRole"
    }]
  })

  tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-codebuild-role"
  })
}

data "aws_iam_policy_document" "codebuild" {
  statement {
    sid    = "S3BuildContextRead"
    effect = "Allow"
    actions = [
      "s3:GetObject",
      "s3:GetObjectVersion",
      "s3:ListBucket",
    ]
    resources = [
      aws_s3_bucket.build_context.arn,
      "${aws_s3_bucket.build_context.arn}/*",
    ]
  }

  statement {
    sid    = "ECRPushPull"
    effect = "Allow"
    actions = [
      "ecr:GetAuthorizationToken",
      "ecr:BatchCheckLayerAvailability",
      "ecr:GetDownloadUrlForLayer",
      "ecr:BatchGetImage",
      "ecr:InitiateLayerUpload",
      "ecr:UploadLayerPart",
      "ecr:CompleteLayerUpload",
      "ecr:PutImage",
      "ecr:DescribeRepositories",
      "ecr:DescribeImages",
    ]
    resources = ["*"]
  }

  statement {
    sid    = "CloudWatchLogs"
    effect = "Allow"
    actions = [
      "logs:CreateLogGroup",
      "logs:CreateLogStream",
      "logs:PutLogEvents",
    ]
    resources = ["arn:aws:logs:*:*:log-group:/aws/codebuild/${local.name_prefix}-image-builder*"]
  }
}

resource "aws_iam_role_policy" "codebuild" {
  name   = "${local.name_prefix}-codebuild-policy"
  role   = aws_iam_role.codebuild.id
  policy = data.aws_iam_policy_document.codebuild.json
}

# CodeBuild project — generic image builder. The buildspec is
# inlined: read context tarball from S3 (path injected via env var
# `BUILD_CONTEXT_KEY`); extract; `docker login` to ECR; `docker
# build`; `docker push`. Caller controls the dest tag via env var
# `IMAGE_URI` injected at StartBuild time.
#
# `privileged_mode = true` is required for the docker daemon to
# come up inside the build container.
resource "aws_codebuild_project" "image_builder" {
  name          = "${local.name_prefix}-image-builder"
  description   = "Builds container images from S3-staged build contexts; pushes to ECR. Used by runner-Lambda manual rebuilds + sockerless-backend-lambda runtime image-inject."
  service_role  = aws_iam_role.codebuild.arn
  build_timeout = 30

  artifacts {
    type = "NO_ARTIFACTS"
  }

  environment {
    type            = "LINUX_CONTAINER"
    image           = "aws/codebuild/amazonlinux2-x86_64-standard:5.0"
    compute_type    = "BUILD_GENERAL1_MEDIUM"
    privileged_mode = true

    environment_variable {
      name  = "AWS_DEFAULT_REGION"
      value = "eu-west-1"
    }

    environment_variable {
      name  = "AWS_ACCOUNT_ID"
      value = data.aws_caller_identity.current.account_id
    }
  }

  source {
    type      = "NO_SOURCE"
    buildspec = <<-YAML
      version: 0.2
      phases:
        pre_build:
          commands:
            - echo "Downloading build context $${BUILD_CONTEXT_KEY} from s3://${aws_s3_bucket.build_context.bucket}/"
            - aws s3 cp "s3://${aws_s3_bucket.build_context.bucket}/$${BUILD_CONTEXT_KEY}" /tmp/context.tar.gz
            - mkdir -p /tmp/build-context
            - tar -xzf /tmp/context.tar.gz -C /tmp/build-context
            - echo "Logging in to ECR..."
            - aws ecr get-login-password --region $${AWS_DEFAULT_REGION} | docker login --username AWS --password-stdin $${AWS_ACCOUNT_ID}.dkr.ecr.$${AWS_DEFAULT_REGION}.amazonaws.com
        build:
          commands:
            - cd /tmp/build-context
            - echo "Building $${IMAGE_URI} for linux/$${TARGET_PLATFORM:-amd64}"
            - docker buildx create --use --name sockerless-builder --driver docker-container || true
            # Lambda image-mode requires Docker schema 2 manifests, not OCI.
            # `--provenance=false` suppresses the attestation manifest list,
            # `oci-mediatypes=false` flips media types from OCI to Docker
            # schema 2. Without these, `aws lambda update-function-code`
            # rejects the image with InvalidParameterValueException.
            - docker buildx build --platform "linux/$${TARGET_PLATFORM:-amd64}" --provenance=false --output "type=image,name=$${IMAGE_URI},oci-mediatypes=false,push=true" .
        post_build:
          commands:
            - echo "Pushed $${IMAGE_URI}"
    YAML
  }

  logs_config {
    cloudwatch_logs {
      group_name = "/aws/codebuild/${local.name_prefix}-image-builder"
    }
  }

  tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-image-builder"
  })
}

output "codebuild_project_name" {
  value       = aws_codebuild_project.image_builder.name
  description = "Set as SOCKERLESS_CODEBUILD_PROJECT on any sockerless backend that needs runtime image builds."
}

output "build_context_bucket" {
  value       = aws_s3_bucket.build_context.bucket
  description = "Set as SOCKERLESS_BUILD_BUCKET on any sockerless backend that needs runtime image builds."
}
