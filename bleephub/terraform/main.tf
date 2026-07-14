locals {
  common_tags = merge(var.tags, {
    component  = "bleephub"
    managed-by = "terraform"
    service    = var.name
  })
  git_bucket     = "${var.name}-git"
  object_bucket  = "${var.name}-objects"
  startup_bucket = "${var.name}-startup"
  azs            = { for index, az in var.availability_zones : tostring(index) => az }
  dqlite_nodes   = { for index in range(3) : tostring(index) => 9000 + index }
  dqlite_data_paths = {
    "0" = "/dqlite/0"
    "1" = "/dqlite/1"
    "2" = "/dqlite/2"
  }
}

data "aws_caller_identity" "current" {}

resource "aws_vpc" "this" {
  cidr_block           = var.vpc_cidr
  enable_dns_hostnames = true
  enable_dns_support   = true
  tags                 = merge(local.common_tags, { Name = "${var.name}-vpc" })
}

resource "aws_internet_gateway" "this" {
  vpc_id = aws_vpc.this.id
  tags   = merge(local.common_tags, { Name = "${var.name}-igw" })
}

resource "aws_subnet" "public" {
  for_each                = local.azs
  vpc_id                  = aws_vpc.this.id
  availability_zone       = each.value
  cidr_block              = cidrsubnet(var.vpc_cidr, 8, tonumber(each.key))
  map_public_ip_on_launch = true
  tags                    = merge(local.common_tags, { Name = "${var.name}-public-${each.value}" })
}

resource "aws_subnet" "private" {
  for_each          = local.azs
  vpc_id            = aws_vpc.this.id
  availability_zone = each.value
  cidr_block        = cidrsubnet(var.vpc_cidr, 8, 128 + tonumber(each.key))
  tags              = merge(local.common_tags, { Name = "${var.name}-private-${each.value}" })
}

resource "aws_route_table" "public" {
  vpc_id = aws_vpc.this.id
  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.this.id
  }
  tags = merge(local.common_tags, { Name = "${var.name}-public" })
}

resource "aws_route_table_association" "public" {
  for_each       = aws_subnet.public
  subnet_id      = each.value.id
  route_table_id = aws_route_table.public.id
}

resource "aws_route_table" "private" {
  for_each = local.azs
  vpc_id   = aws_vpc.this.id
  tags     = merge(local.common_tags, { Name = "${var.name}-private-${each.value}" })
}

resource "aws_route_table_association" "private" {
  for_each       = aws_subnet.private
  subnet_id      = each.value.id
  route_table_id = aws_route_table.private[each.key].id
}

# fck-nat is the actual upstream NAT-instance implementation. It owns the
# default routes of every private subnet; this module never provisions an AWS
# managed NAT Gateway.
module "fck_nat" {
  source  = "RaJiska/fck-nat/aws"
  version = "1.6.0"

  name                 = "${var.name}-fck-nat"
  vpc_id               = aws_vpc.this.id
  subnet_id            = aws_subnet.public["0"].id
  update_route_tables  = true
  route_tables_ids     = { for key, route_table in aws_route_table.private : key => route_table.id }
  ha_mode              = false
  use_cloudwatch_agent = true
  tags                 = local.common_tags
}

resource "aws_vpc_endpoint" "s3" {
  vpc_id            = aws_vpc.this.id
  service_name      = "com.amazonaws.${var.region}.s3"
  vpc_endpoint_type = "Gateway"
  route_table_ids   = [for route_table in aws_route_table.private : route_table.id]
  tags              = merge(local.common_tags, { Name = "${var.name}-s3" })
}

resource "aws_s3_bucket" "git" {
  bucket = local.git_bucket
  tags   = local.common_tags
}

resource "aws_s3_bucket_versioning" "git" {
  bucket = aws_s3_bucket.git.id
  versioning_configuration { status = "Suspended" }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "git" {
  bucket = aws_s3_bucket.git.id
  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

resource "aws_s3_bucket" "objects" {
  bucket = local.object_bucket
  tags   = local.common_tags
}

resource "aws_s3_bucket_versioning" "objects" {
  bucket = aws_s3_bucket.objects.id
  versioning_configuration { status = "Enabled" }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "objects" {
  bucket = aws_s3_bucket.objects.id
  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

# This contains exactly one public, non-sensitive document. It makes the
# scale-to-zero transition visible without starting an ECS task merely to
# render a loading screen; no application, Git, object, or status data is
# readable from this bucket.
resource "aws_s3_bucket" "startup" {
  bucket = local.startup_bucket
  tags   = local.common_tags
}

resource "aws_s3_bucket_public_access_block" "startup" {
  bucket                  = aws_s3_bucket.startup.id
  block_public_acls       = true
  ignore_public_acls      = true
  block_public_policy     = false
  restrict_public_buckets = false
}

data "aws_iam_policy_document" "startup_public_read" {
  statement {
    sid       = "ReadStartupDocumentOnly"
    effect    = "Allow"
    actions   = ["s3:GetObject"]
    resources = ["${aws_s3_bucket.startup.arn}/startup/index.html"]
    principals {
      type        = "*"
      identifiers = ["*"]
    }
  }
}

resource "aws_s3_bucket_policy" "startup_public_read" {
  bucket     = aws_s3_bucket.startup.id
  policy     = data.aws_iam_policy_document.startup_public_read.json
  depends_on = [aws_s3_bucket_public_access_block.startup]
}

resource "aws_s3_object" "startup_page" {
  bucket        = aws_s3_bucket.startup.id
  key           = "startup/index.html"
  source        = var.startup_page_path
  etag          = filemd5(var.startup_page_path)
  content_type  = "text/html; charset=utf-8"
  cache_control = "no-store, max-age=0"
  tags          = local.common_tags
}

resource "aws_secretsmanager_secret" "admin_token" {
  name                    = "${var.name}/admin-token"
  recovery_window_in_days = 7
  tags                    = local.common_tags
}

resource "aws_secretsmanager_secret_version" "admin_token" {
  secret_id     = aws_secretsmanager_secret.admin_token.id
  secret_string = var.admin_token
}

resource "tls_private_key" "ssh_host" {
  algorithm = "ED25519"
}

resource "aws_secretsmanager_secret" "ssh_host_key" {
  name                    = "${var.name}/ssh-host-key"
  recovery_window_in_days = 7
  tags                    = local.common_tags
}

resource "aws_secretsmanager_secret_version" "ssh_host_key" {
  secret_id     = aws_secretsmanager_secret.ssh_host_key.id
  secret_string = tls_private_key.ssh_host.private_key_openssh
}

resource "aws_cloudwatch_log_group" "this" {
  name              = "/bleephub/${var.name}"
  retention_in_days = var.log_retention_days
  tags              = local.common_tags
}

resource "aws_security_group" "api_link" {
  name_prefix = "${var.name}-api-link-"
  vpc_id      = aws_vpc.this.id
  egress {
    protocol    = "-1"
    from_port   = 0
    to_port     = 0
    cidr_blocks = ["0.0.0.0/0"]
  }
  tags = local.common_tags
}

resource "aws_security_group" "task" {
  name_prefix = "${var.name}-task-"
  vpc_id      = aws_vpc.this.id
  ingress {
    protocol    = "tcp"
    from_port   = 5555
    to_port     = 5555
    cidr_blocks = [var.vpc_cidr]
  }
  ingress {
    protocol    = "tcp"
    from_port   = 2222
    to_port     = 2222
    cidr_blocks = [var.vpc_cidr]
  }
  egress {
    protocol    = "-1"
    from_port   = 0
    to_port     = 0
    cidr_blocks = ["0.0.0.0/0"]
  }
  tags = local.common_tags
}

resource "aws_security_group" "ssh" {
  name_prefix = "${var.name}-ssh-"
  vpc_id      = aws_vpc.this.id
  ingress {
    protocol    = "tcp"
    from_port   = 22
    to_port     = 22
    cidr_blocks = ["0.0.0.0/0"]
  }
  egress {
    protocol    = "-1"
    from_port   = 0
    to_port     = 0
    cidr_blocks = ["0.0.0.0/0"]
  }
  tags = local.common_tags
}

resource "aws_security_group" "ssh_gateway" {
  name_prefix = "${var.name}-ssh-gateway-"
  vpc_id      = aws_vpc.this.id
  ingress {
    protocol        = "tcp"
    from_port       = 2222
    to_port         = 2222
    security_groups = [aws_security_group.ssh.id]
  }
  egress {
    protocol    = "-1"
    from_port   = 0
    to_port     = 0
    cidr_blocks = ["0.0.0.0/0"]
  }
  tags = local.common_tags
}

resource "aws_security_group" "efs" {
  name_prefix = "${var.name}-efs-"
  vpc_id      = aws_vpc.this.id
  ingress {
    protocol        = "tcp"
    from_port       = 2049
    to_port         = 2049
    security_groups = [aws_security_group.task.id, aws_security_group.dqlite.id]
  }
  tags = local.common_tags
}

resource "aws_security_group" "dqlite" {
  name_prefix = "${var.name}-dqlite-"
  vpc_id      = aws_vpc.this.id
  ingress {
    protocol    = "tcp"
    from_port   = 9000
    to_port     = 9000
    cidr_blocks = [var.vpc_cidr]
  }
  egress {
    protocol    = "-1"
    from_port   = 0
    to_port     = 0
    cidr_blocks = ["0.0.0.0/0"]
  }
  tags = local.common_tags
}

resource "aws_efs_file_system" "sqlite" {
  encrypted = true
  tags      = merge(local.common_tags, { Name = "${var.name}-sqlite" })
}

resource "aws_efs_access_point" "sqlite" {
  file_system_id = aws_efs_file_system.sqlite.id
  posix_user {
    uid = 0
    gid = 0
  }
  root_directory {
    path = "/bleephub"
    creation_info {
      owner_uid   = 0
      owner_gid   = 0
      permissions = "0700"
    }
  }
  tags = local.common_tags
}

resource "aws_efs_access_point" "dqlite" {
  for_each       = local.dqlite_nodes
  file_system_id = aws_efs_file_system.sqlite.id
  posix_user {
    uid = 0
    gid = 0
  }
  root_directory {
    path = local.dqlite_data_paths[each.key]
    creation_info {
      owner_uid   = 0
      owner_gid   = 0
      permissions = "0700"
    }
  }
  tags = merge(local.common_tags, { Name = "${var.name}-dqlite-${each.key}" })
}

resource "aws_efs_mount_target" "sqlite" {
  for_each        = aws_subnet.private
  file_system_id  = aws_efs_file_system.sqlite.id
  subnet_id       = each.value.id
  security_groups = [aws_security_group.efs.id]
}

data "aws_iam_policy_document" "assume_ecs" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["ecs-tasks.amazonaws.com"]
    }
  }
}

data "aws_iam_policy_document" "assume_lambda" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["lambda.amazonaws.com"]
    }
  }
}

data "aws_iam_policy_document" "assume_scheduler" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["scheduler.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "execution" {
  name_prefix        = "${var.name}-execution-"
  assume_role_policy = data.aws_iam_policy_document.assume_ecs.json
  tags               = local.common_tags
}

resource "aws_iam_role_policy_attachment" "execution" {
  role       = aws_iam_role.execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

resource "aws_iam_role_policy" "execution_secret" {
  name   = "read-admin-token"
  role   = aws_iam_role.execution.id
  policy = jsonencode({ Version = "2012-10-17", Statement = [{ Effect = "Allow", Action = ["secretsmanager:GetSecretValue"], Resource = compact([aws_secretsmanager_secret.admin_token.arn, aws_secretsmanager_secret.ssh_host_key.arn, var.github_oauth_client_secret_arn]) }] })
}

resource "aws_iam_role" "task" {
  name_prefix        = "${var.name}-task-"
  assume_role_policy = data.aws_iam_policy_document.assume_ecs.json
  tags               = local.common_tags
}

resource "aws_iam_role_policy" "task_storage" {
  name = "bleephub-durable-storage"
  role = aws_iam_role.task.id
  policy = jsonencode({ Version = "2012-10-17", Statement = [
    { Effect = "Allow", Action = ["s3:ListBucket"], Resource = [aws_s3_bucket.git.arn, aws_s3_bucket.objects.arn] },
    { Effect = "Allow", Action = ["s3:GetObject", "s3:PutObject", "s3:DeleteObject"], Resource = ["${aws_s3_bucket.git.arn}/*", "${aws_s3_bucket.objects.arn}/*"] }
  ] })
}

resource "aws_iam_role" "ssh_gateway" {
  name_prefix        = "${var.name}-ssh-gateway-"
  assume_role_policy = data.aws_iam_policy_document.assume_ecs.json
  tags               = local.common_tags
}

resource "aws_iam_role" "wake" {
  name_prefix        = "${var.name}-wake-"
  assume_role_policy = data.aws_iam_policy_document.assume_lambda.json
  tags               = local.common_tags
}

resource "aws_iam_role" "idle_arm_scheduler" {
  name_prefix        = "${var.name}-idle-arm-"
  assume_role_policy = data.aws_iam_policy_document.assume_scheduler.json
  tags               = local.common_tags
}

resource "aws_iam_role_policy" "idle_arm_scheduler" {
  name = "invoke-idle-arm"
  role = aws_iam_role.idle_arm_scheduler.id
  policy = jsonencode({
    Version   = "2012-10-17"
    Statement = [{ Effect = "Allow", Action = ["lambda:InvokeFunction"], Resource = aws_lambda_function.idle_arm.arn }]
  })
}

resource "aws_iam_role_policy_attachment" "wake_logs" {
  role       = aws_iam_role.wake.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
}

resource "aws_iam_role_policy" "wake_service" {
  name = "wake-bleephub-service"
  role = aws_iam_role.wake.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      { Effect = "Allow", Action = ["ecs:DescribeServices", "ecs:UpdateService"], Resource = concat([aws_ecs_service.this.id], [for service in aws_ecs_service.dqlite : service.id]) },
      { Effect = "Allow", Action = ["ecs:ListTasks", "ecs:DescribeTasks", "ecs:StopTask"], Resource = "*" },
      { Effect = "Allow", Action = ["elasticloadbalancing:DescribeTargetHealth"], Resource = "*" },
      { Effect = "Allow", Action = ["secretsmanager:GetSecretValue"], Resource = aws_secretsmanager_secret.admin_token.arn },
      { Effect = "Allow", Action = ["apigateway:GET", "apigateway:PATCH"], Resource = "arn:aws:apigateway:${var.region}::/apis/${aws_apigatewayv2_api.this.id}/*" },
      { Effect = "Allow", Action = ["cloudwatch:SetAlarmState", "cloudwatch:DisableAlarmActions", "cloudwatch:EnableAlarmActions"], Resource = aws_cloudwatch_metric_alarm.idle_shutdown.arn },
      { Effect = "Allow", Action = ["cloudwatch:DescribeAlarms"], Resource = "*" },
      { Effect = "Allow", Action = ["scheduler:CreateSchedule", "scheduler:UpdateSchedule"], Resource = "arn:aws:scheduler:${var.region}:${data.aws_caller_identity.current.account_id}:schedule/default/${var.name}-idle-arm" },
      { Effect = "Allow", Action = ["lambda:InvokeFunction"], Resource = aws_lambda_function.idle_shutdown.arn },
      { Effect = "Allow", Action = ["iam:PassRole"], Resource = aws_iam_role.idle_arm_scheduler.arn, Condition = { StringEquals = { "iam:PassedToService" = "scheduler.amazonaws.com" } } }
    ]
  })
}

resource "aws_ecs_cluster" "this" {
  name = var.name
  tags = local.common_tags
}

resource "aws_acm_certificate" "this" {
  domain_name               = var.domain_name
  subject_alternative_names = ["admin.${var.domain_name}"]
  validation_method         = "DNS"
  tags                      = local.common_tags
  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_route53_record" "certificate" {
  for_each = { for option in aws_acm_certificate.this.domain_validation_options : option.domain_name => option }
  zone_id  = var.hosted_zone_id
  name     = each.value.resource_record_name
  type     = each.value.resource_record_type
  records  = [each.value.resource_record_value]
  ttl      = 60
}

resource "aws_acm_certificate_validation" "this" {
  certificate_arn         = aws_acm_certificate.this.arn
  validation_record_fqdns = [for record in aws_route53_record.certificate : record.fqdn]
}

resource "aws_lb" "this" {
  name               = "${var.name}-private"
  internal           = true
  load_balancer_type = "network"
  # API Gateway's VPC link creates network interfaces in every private subnet,
  # while the scale-to-zero application service intentionally runs one task.
  # Cross-zone forwarding keeps either link interface able to reach that task.
  enable_cross_zone_load_balancing = true
  subnets                          = [for subnet in aws_subnet.private : subnet.id]
  tags                             = local.common_tags
}

resource "aws_lb" "ssh" {
  name               = "${var.name}-ssh"
  internal           = false
  load_balancer_type = "network"
  security_groups    = [aws_security_group.ssh.id]
  subnets            = [for subnet in aws_subnet.public : subnet.id]
  tags               = local.common_tags
}

resource "aws_lb_target_group" "ssh" {
  name        = "${var.name}-ssh"
  port        = 2222
  protocol    = "TCP"
  target_type = "ip"
  vpc_id      = aws_vpc.this.id
  health_check {
    protocol = "TCP"
    matcher  = null
  }
  tags = local.common_tags
}

resource "aws_lb_target_group" "app_ssh" {
  name        = "${var.name}-app-ssh"
  port        = 2222
  protocol    = "TCP"
  target_type = "ip"
  vpc_id      = aws_vpc.this.id
  health_check { protocol = "TCP" }
  tags = local.common_tags
}

resource "aws_lb_listener" "ssh" {
  load_balancer_arn = aws_lb.ssh.arn
  port              = 22
  protocol          = "TCP"
  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.ssh.arn
  }
}

resource "aws_lb_target_group" "this" {
  name        = var.name
  port        = 5555
  protocol    = "HTTP"
  target_type = "ip"
  vpc_id      = aws_vpc.this.id
  health_check {
    path    = "/health"
    matcher = "200"
  }
  tags = local.common_tags
}

resource "aws_lb_target_group" "private" {
  name        = "${var.name}-private"
  port        = 5555
  protocol    = "TCP"
  target_type = "ip"
  vpc_id      = aws_vpc.this.id
  health_check { protocol = "TCP" }
  tags = local.common_tags
}

resource "aws_lb_listener" "private_http" {
  load_balancer_arn = aws_lb.this.arn
  port              = 5555
  protocol          = "TCP"
  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.private.arn
  }
}

resource "aws_lb_listener" "private_ssh" {
  load_balancer_arn = aws_lb.this.arn
  port              = 2222
  protocol          = "TCP"
  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.app_ssh.arn
  }
}

resource "aws_lb_target_group" "dqlite" {
  for_each    = local.dqlite_nodes
  name        = "${var.name}-dqlite-${each.key}"
  port        = 9000
  protocol    = "TCP"
  target_type = "ip"
  vpc_id      = aws_vpc.this.id
  # The idle controller has already drained the Bleephub application before
  # stopping a voter. Keeping a Raft voter registered for five more minutes
  # leaves its exclusive durable-directory lock behind during the next wake.
  deregistration_delay = 5
  health_check {
    protocol            = "HTTP"
    path                = "/health"
    matcher             = "200"
    interval            = 5
    timeout             = 2
    healthy_threshold   = 2
    unhealthy_threshold = 2
  }
  tags = merge(local.common_tags, { Name = "${var.name}-dqlite-${each.key}" })
}

resource "aws_lb_listener" "dqlite" {
  for_each          = local.dqlite_nodes
  load_balancer_arn = aws_lb.this.arn
  port              = each.value
  protocol          = "TCP"
  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.dqlite[each.key].arn
  }
}

resource "aws_apigatewayv2_vpc_link" "this" {
  name               = var.name
  security_group_ids = [aws_security_group.api_link.id]
  subnet_ids         = [for subnet in aws_subnet.private : subnet.id]
  tags               = local.common_tags
}

resource "aws_apigatewayv2_api" "this" {
  name          = var.name
  protocol_type = "HTTP"
  tags          = local.common_tags
}

resource "aws_apigatewayv2_integration" "service" {
  api_id                 = aws_apigatewayv2_api.this.id
  integration_type       = "HTTP_PROXY"
  integration_uri        = aws_lb_listener.private_http.arn
  integration_method     = "ANY"
  connection_type        = "VPC_LINK"
  connection_id          = aws_apigatewayv2_vpc_link.this.id
  payload_format_version = "1.0"
}

resource "aws_apigatewayv2_integration" "wake" {
  api_id                 = aws_apigatewayv2_api.this.id
  integration_type       = "AWS_PROXY"
  integration_uri        = aws_lambda_function.wake.invoke_arn
  payload_format_version = "2.0"
}

resource "aws_apigatewayv2_integration" "startup_page" {
  api_id                 = aws_apigatewayv2_api.this.id
  integration_type       = "HTTP_PROXY"
  integration_uri        = "https://${aws_s3_bucket.startup.bucket_regional_domain_name}/startup/index.html"
  integration_method     = "GET"
  payload_format_version = "1.0"
}

resource "aws_apigatewayv2_route" "startup_page" {
  api_id    = aws_apigatewayv2_api.this.id
  route_key = "GET /__startup"
  target    = "integrations/${aws_apigatewayv2_integration.startup_page.id}"
}

resource "aws_apigatewayv2_route" "startup_status" {
  api_id    = aws_apigatewayv2_api.this.id
  route_key = "GET /__startup/status"
  target    = "integrations/${aws_apigatewayv2_integration.wake.id}"
}

resource "aws_apigatewayv2_route" "default" {
  api_id    = aws_apigatewayv2_api.this.id
  route_key = "$default"
  target    = "integrations/${aws_apigatewayv2_integration.wake.id}"
  lifecycle { ignore_changes = [target] }
}

resource "aws_apigatewayv2_stage" "default" {
  api_id      = aws_apigatewayv2_api.this.id
  name        = "$default"
  auto_deploy = true
  default_route_settings {
    throttling_burst_limit = 100
    throttling_rate_limit  = 50
  }
  tags = local.common_tags
}

resource "aws_lambda_permission" "wake_api" {
  statement_id  = "allow-api-gateway-wake"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.wake.function_name
  principal     = "apigateway.amazonaws.com"
  source_arn    = "${aws_apigatewayv2_api.this.execution_arn}/*/*"
}

resource "aws_apigatewayv2_domain_name" "service" {
  domain_name = var.domain_name
  domain_name_configuration {
    certificate_arn = aws_acm_certificate_validation.this.certificate_arn
    endpoint_type   = "REGIONAL"
    security_policy = "TLS_1_2"
  }
}

resource "aws_apigatewayv2_domain_name" "admin" {
  domain_name = "admin.${var.domain_name}"
  domain_name_configuration {
    certificate_arn = aws_acm_certificate_validation.this.certificate_arn
    endpoint_type   = "REGIONAL"
    security_policy = "TLS_1_2"
  }
}

resource "aws_apigatewayv2_api_mapping" "service" {
  api_id      = aws_apigatewayv2_api.this.id
  domain_name = aws_apigatewayv2_domain_name.service.id
  stage       = aws_apigatewayv2_stage.default.id
}

resource "aws_apigatewayv2_api_mapping" "admin" {
  api_id      = aws_apigatewayv2_api.this.id
  domain_name = aws_apigatewayv2_domain_name.admin.id
  stage       = aws_apigatewayv2_stage.default.id
}

resource "aws_route53_record" "service" {
  zone_id = var.hosted_zone_id
  name    = var.domain_name
  type    = "A"
  alias {
    name                   = aws_apigatewayv2_domain_name.service.domain_name_configuration[0].target_domain_name
    zone_id                = aws_apigatewayv2_domain_name.service.domain_name_configuration[0].hosted_zone_id
    evaluate_target_health = false
  }
}

resource "aws_route53_record" "admin" {
  zone_id = var.hosted_zone_id
  name    = "admin.${var.domain_name}"
  type    = "A"
  alias {
    name                   = aws_apigatewayv2_domain_name.admin.domain_name_configuration[0].target_domain_name
    zone_id                = aws_apigatewayv2_domain_name.admin.domain_name_configuration[0].hosted_zone_id
    evaluate_target_health = false
  }
}

resource "aws_route53_record" "ssh" {
  zone_id = var.hosted_zone_id
  name    = "ssh.${var.domain_name}"
  type    = "A"
  alias {
    name                   = aws_lb.ssh.dns_name
    zone_id                = aws_lb.ssh.zone_id
    evaluate_target_health = true
  }
}

resource "aws_ecs_task_definition" "this" {
  family                   = var.name
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = tostring(var.task_cpu)
  memory                   = tostring(var.task_memory)
  runtime_platform {
    cpu_architecture        = "ARM64"
    operating_system_family = "LINUX"
  }
  execution_role_arn = aws_iam_role.execution.arn
  task_role_arn      = aws_iam_role.task.arn
  volume {
    name = "sqlite"
    efs_volume_configuration {
      file_system_id     = aws_efs_file_system.sqlite.id
      transit_encryption = "ENABLED"
      authorization_config {
        access_point_id = aws_efs_access_point.sqlite.id
        iam             = "DISABLED"
      }
    }
  }
  container_definitions = jsonencode([{ name = "bleephub", image = var.container_image, essential = true, portMappings = [{ containerPort = 5555, protocol = "tcp" }, { containerPort = 2222, protocol = "tcp" }], mountPoints = [{ sourceVolume = "sqlite", containerPath = "/var/lib/bleephub", readOnly = false }], environment = concat([{ name = "BLEEPHUB_PERSIST", value = "true" }, { name = "BLEEPHUB_DATA_DIR", value = "/var/lib/bleephub" }, { name = "BLEEPHUB_DQLITE_SERVERS", value = join(",", [for node, port in local.dqlite_nodes : "${aws_lb.this.dns_name}:${port}"]) }, { name = "BLEEPHUB_S3_BUCKET", value = aws_s3_bucket.git.bucket }, { name = "BLEEPHUB_S3_PREFIX", value = "git" }, { name = "BLEEPHUB_OBJECT_S3_BUCKET", value = aws_s3_bucket.objects.bucket }, { name = "BLEEPHUB_OBJECT_S3_PREFIX", value = "objects" }, { name = "BLEEPHUB_S3_REGION", value = var.region }, { name = "BLEEPHUB_EXTERNAL_URL", value = "https://${var.domain_name}" }, { name = "BLEEPHUB_ADMIN_HOST", value = "admin.${var.domain_name}" }, { name = "BLEEPHUB_SSH_ADDR", value = ":2222" }, { name = "BLEEPHUB_SSH_HOST", value = "ssh.${var.domain_name}" }], var.github_oauth_client_id == "" ? [] : [{ name = "BLEEPHUB_GITHUB_OAUTH_CLIENT_ID", value = var.github_oauth_client_id }]), secrets = concat([{ name = "BLEEPHUB_ADMIN_TOKEN", valueFrom = aws_secretsmanager_secret.admin_token.arn }, { name = "BLEEPHUB_SSH_HOST_KEY", valueFrom = aws_secretsmanager_secret.ssh_host_key.arn }], var.github_oauth_client_secret_arn == "" ? [] : [{ name = "BLEEPHUB_GITHUB_OAUTH_CLIENT_SECRET", valueFrom = var.github_oauth_client_secret_arn }]), logConfiguration = { logDriver = "awslogs", options = { awslogs-group = aws_cloudwatch_log_group.this.name, awslogs-region = var.region, awslogs-stream-prefix = "service" } } }])
  tags                  = local.common_tags
}

resource "aws_ecs_task_definition" "ssh_gateway" {
  family                   = "${var.name}-ssh-gateway"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = "256"
  memory                   = "512"
  runtime_platform {
    cpu_architecture        = "ARM64"
    operating_system_family = "LINUX"
  }
  execution_role_arn = aws_iam_role.execution.arn
  task_role_arn      = aws_iam_role.ssh_gateway.arn
  container_definitions = jsonencode([{
    name         = "ssh-gateway"
    image        = var.container_image
    essential    = true
    entryPoint   = ["/usr/local/bin/bleephub-ssh-gateway"]
    portMappings = [{ containerPort = 2222, protocol = "tcp" }]
    environment = [
      { name = "BLEEPHUB_WAKE_URL", value = "https://${var.domain_name}/health" },
      { name = "BLEEPHUB_INTERNAL_SSH_TARGET", value = "${aws_lb.this.dns_name}:2222" }
    ]
    logConfiguration = { logDriver = "awslogs", options = { awslogs-group = aws_cloudwatch_log_group.this.name, awslogs-region = var.region, awslogs-stream-prefix = "ssh-gateway" } }
  }])
  tags = local.common_tags
}

resource "aws_ecs_task_definition" "dqlite" {
  for_each                 = local.dqlite_nodes
  family                   = "${var.name}-dqlite-${each.key}"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = "512"
  memory                   = "1024"
  runtime_platform {
    cpu_architecture        = "ARM64"
    operating_system_family = "LINUX"
  }
  execution_role_arn = aws_iam_role.execution.arn
  task_role_arn      = aws_iam_role.task.arn
  volume {
    name = "dqlite"
    efs_volume_configuration {
      file_system_id     = aws_efs_file_system.sqlite.id
      transit_encryption = "ENABLED"
      authorization_config {
        access_point_id = aws_efs_access_point.dqlite[each.key].id
        iam             = "DISABLED"
      }
    }
  }
  container_definitions = jsonencode([{
    name         = "dqlite"
    image        = var.container_image
    essential    = true
    entryPoint   = ["/usr/local/bin/bleephub-dqlite-node"]
    portMappings = [{ containerPort = 9000, protocol = "tcp" }]
    mountPoints  = [{ sourceVolume = "dqlite", containerPath = "/var/lib/dqlite", readOnly = false }]
    environment = concat([
      { name = "BLEEPHUB_DQLITE_DATA_DIR", value = "/var/lib/dqlite" },
      { name = "BLEEPHUB_DQLITE_ADVERTISE_ADDR", value = "${aws_lb.this.dns_name}:${each.value}" }
    ], each.key == "0" ? [] : [{ name = "BLEEPHUB_DQLITE_JOIN", value = "${aws_lb.this.dns_name}:${local.dqlite_nodes["0"]}" }])
    logConfiguration = { logDriver = "awslogs", options = { awslogs-group = aws_cloudwatch_log_group.this.name, awslogs-region = var.region, awslogs-stream-prefix = "dqlite-${each.key}" } }
  }])
  tags = merge(local.common_tags, { Name = "${var.name}-dqlite-${each.key}" })
}

resource "aws_ecs_service" "this" {
  name            = var.name
  cluster         = aws_ecs_cluster.this.arn
  task_definition = aws_ecs_task_definition.this.arn
  desired_count   = 0
  launch_type     = "FARGATE"

  # Artifact/package files retain one mounted writer while metadata is served
  # by the independent dqlite quorum.
  deployment_minimum_healthy_percent = 0
  deployment_maximum_percent         = 100
  availability_zone_rebalancing      = "DISABLED"

  network_configuration {
    subnets          = [for subnet in aws_subnet.private : subnet.id]
    security_groups  = [aws_security_group.task.id]
    assign_public_ip = false
  }
  load_balancer {
    target_group_arn = aws_lb_target_group.private.arn
    container_name   = "bleephub"
    container_port   = 5555
  }
  load_balancer {
    target_group_arn = aws_lb_target_group.app_ssh.arn
    container_name   = "bleephub"
    container_port   = 2222
  }
  depends_on = [aws_lb_listener.private_http, aws_efs_mount_target.sqlite, module.fck_nat]
  tags       = local.common_tags
  lifecycle {
    # The wake and idle Lambdas own runtime capacity; Terraform establishes
    # zero only when creating the service and must not interrupt live work.
    ignore_changes = [desired_count]
  }
}

resource "aws_ecs_service" "dqlite" {
  for_each        = local.dqlite_nodes
  name            = "${var.name}-dqlite-${each.key}"
  cluster         = aws_ecs_cluster.this.arn
  task_definition = aws_ecs_task_definition.dqlite[each.key].arn
  desired_count   = 0
  launch_type     = "FARGATE"
  # A voter cannot report /health until it has joined the durable Raft quorum.
  # Give that real recovery work time to complete before ECS reacts to the
  # Network Load Balancer's intentionally strict readiness probe.
  health_check_grace_period_seconds  = 300
  deployment_minimum_healthy_percent = 0
  deployment_maximum_percent         = 100
  availability_zone_rebalancing      = "DISABLED"
  network_configuration {
    subnets          = [for subnet in aws_subnet.private : subnet.id]
    security_groups  = [aws_security_group.dqlite.id]
    assign_public_ip = false
  }
  load_balancer {
    target_group_arn = aws_lb_target_group.dqlite[each.key].arn
    container_name   = "dqlite"
    container_port   = 9000
  }
  depends_on = [aws_lb_listener.dqlite, aws_efs_mount_target.sqlite, module.fck_nat]
  tags       = merge(local.common_tags, { Name = "${var.name}-dqlite-${each.key}" })
  lifecycle {
    # Each voter is started and stopped with the application by the wake path.
    ignore_changes = [desired_count]
  }
}

resource "aws_ecs_service" "ssh_gateway" {
  name            = "${var.name}-ssh-gateway"
  cluster         = aws_ecs_cluster.this.arn
  task_definition = aws_ecs_task_definition.ssh_gateway.arn
  desired_count   = 1
  launch_type     = "FARGATE"
  network_configuration {
    subnets          = [for subnet in aws_subnet.private : subnet.id]
    security_groups  = [aws_security_group.ssh_gateway.id]
    assign_public_ip = false
  }
  load_balancer {
    target_group_arn = aws_lb_target_group.ssh.arn
    container_name   = "ssh-gateway"
    container_port   = 2222
  }
  depends_on = [aws_lb_listener.ssh, module.fck_nat]
  tags       = local.common_tags
}

resource "aws_lambda_function" "wake" {
  filename         = var.wake_listener_zip_path
  function_name    = "${var.name}-wake"
  role             = aws_iam_role.wake.arn
  handler          = "bootstrap"
  runtime          = "provided.al2023"
  architectures    = ["arm64"]
  source_code_hash = filebase64sha256(var.wake_listener_zip_path)
  timeout          = 120

  environment {
    variables = {
      ECS_CLUSTER                 = aws_ecs_cluster.this.name
      ECS_SERVICE                 = aws_ecs_service.this.name
      API_ID                      = aws_apigatewayv2_api.this.id
      SERVICE_INTEGRATION_ID      = aws_apigatewayv2_integration.service.id
      SERVICE_TARGET_GROUP_ARN    = aws_lb_target_group.private.arn
      DQLITE_SERVICES             = join(",", [for node in sort(keys(local.dqlite_nodes)) : aws_ecs_service.dqlite[node].name])
      DQLITE_TARGET_GROUP_ARNS    = join(",", [for node in sort(keys(local.dqlite_nodes)) : aws_lb_target_group.dqlite[node].arn])
      IDLE_ALARM_NAME             = aws_cloudwatch_metric_alarm.idle_shutdown.alarm_name
      IDLE_ARM_FUNCTION_ARN       = aws_lambda_function.idle_arm.arn
      IDLE_ARM_SCHEDULER_ROLE_ARN = aws_iam_role.idle_arm_scheduler.arn
      IDLE_ARM_SCHEDULE_NAME      = "${var.name}-idle-arm"
      ADMIN_TOKEN_SECRET_ARN      = aws_secretsmanager_secret.admin_token.arn
    }
  }

  tags = local.common_tags
}

resource "aws_cloudwatch_metric_alarm" "idle_shutdown" {
  alarm_name          = "${var.name}-five-minute-idle"
  alarm_description   = "Stops Bleephub after five minutes without Amazon API Gateway requests."
  comparison_operator = "LessThanOrEqualToThreshold"
  evaluation_periods  = var.idle_shutdown_minutes
  metric_name         = "Count"
  namespace           = "AWS/ApiGateway"
  period              = 60
  statistic           = "Sum"
  threshold           = 0
  treat_missing_data  = "breaching"
  # The wake controller enables this only after the API Gateway request that
  # woke the stack is visible to CloudWatch, avoiding a stale zero window.
  actions_enabled = false
  alarm_actions   = [aws_lambda_function.idle_shutdown.arn]

  lifecycle {
    ignore_changes = [actions_enabled]
  }

  dimensions = {
    ApiId = aws_apigatewayv2_api.this.id
    Stage = aws_apigatewayv2_stage.default.name
  }

  tags = local.common_tags
}

resource "aws_lambda_function" "idle_shutdown" {
  filename         = var.wake_listener_zip_path
  function_name    = "${var.name}-idle-shutdown"
  role             = aws_iam_role.wake.arn
  handler          = "bootstrap"
  runtime          = "provided.al2023"
  architectures    = ["arm64"]
  source_code_hash = filebase64sha256(var.wake_listener_zip_path)
  timeout          = 300

  environment {
    variables = {
      ECS_CLUSTER              = aws_ecs_cluster.this.name
      ECS_SERVICE              = aws_ecs_service.this.name
      IDLE_SHUTDOWN            = "true"
      IDLE_ALARM_NAME          = "${var.name}-five-minute-idle"
      API_ID                   = aws_apigatewayv2_api.this.id
      SERVICE_INTEGRATION_ID   = aws_apigatewayv2_integration.service.id
      SERVICE_TARGET_GROUP_ARN = aws_lb_target_group.private.arn
      DQLITE_SERVICES          = join(",", [for node in sort(keys(local.dqlite_nodes)) : aws_ecs_service.dqlite[node].name])
      DQLITE_TARGET_GROUP_ARNS = join(",", [for node in sort(keys(local.dqlite_nodes)) : aws_lb_target_group.dqlite[node].arn])
    }
  }

  tags = local.common_tags
}

resource "aws_lambda_function" "idle_arm" {
  filename         = var.wake_listener_zip_path
  function_name    = "${var.name}-idle-arm"
  role             = aws_iam_role.wake.arn
  handler          = "bootstrap"
  runtime          = "provided.al2023"
  architectures    = ["arm64"]
  source_code_hash = filebase64sha256(var.wake_listener_zip_path)
  timeout          = 30

  environment {
    variables = {
      IDLE_ARM                   = "true"
      IDLE_ALARM_NAME            = aws_cloudwatch_metric_alarm.idle_shutdown.alarm_name
      IDLE_SHUTDOWN_FUNCTION_ARN = aws_lambda_function.idle_shutdown.arn
    }
  }

  tags = local.common_tags
}

resource "aws_lambda_permission" "idle_shutdown_alarm" {
  statement_id  = "allow-cloudwatch-idle-shutdown"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.idle_shutdown.function_name
  principal     = "lambda.alarms.cloudwatch.amazonaws.com"
  source_arn    = aws_cloudwatch_metric_alarm.idle_shutdown.arn
}
