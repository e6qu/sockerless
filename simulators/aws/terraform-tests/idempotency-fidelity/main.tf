terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "6.47.0"
    }
  }
}

variable "endpoint" {
  type = string
}

provider "aws" {
  region                      = "us-east-1"
  access_key                  = "test"
  secret_key                  = "test"
  skip_credentials_validation = true
  skip_requesting_account_id  = true

  endpoints {
    ec2      = var.endpoint
    ecs      = var.endpoint
    elbv2    = var.endpoint
    logs     = var.endpoint
    dynamodb = var.endpoint
    ecr      = var.endpoint
    acm      = var.endpoint
  }
}

resource "aws_vpc" "main" {
  cidr_block = "10.91.0.0/16"
}

resource "aws_subnet" "a" {
  vpc_id            = aws_vpc.main.id
  cidr_block        = "10.91.1.0/24"
  availability_zone = "us-east-1a"
}

resource "aws_subnet" "b" {
  vpc_id            = aws_vpc.main.id
  cidr_block        = "10.91.2.0/24"
  availability_zone = "us-east-1b"
}

resource "aws_security_group" "alb" {
  name   = "fidelity-alb"
  vpc_id = aws_vpc.main.id
}

resource "aws_security_group" "tasks" {
  name   = "fidelity-tasks"
  vpc_id = aws_vpc.main.id
}

# All-traffic egress: ip_protocol="-1" carries no ports. The provider reads
# from_port/to_port back as null; a sim returning 0 drifts "0 -> null".
resource "aws_vpc_security_group_egress_rule" "tasks_all" {
  security_group_id = aws_security_group.tasks.id
  ip_protocol       = "-1"
  cidr_ipv4         = "0.0.0.0/0"
}

# Referencing rule: referenced_security_group_id must read back as the bare
# sg-id (no account prefix), or it drifts every plan.
resource "aws_vpc_security_group_ingress_rule" "tasks_from_alb" {
  security_group_id            = aws_security_group.tasks.id
  ip_protocol                  = "tcp"
  from_port                    = 3000
  to_port                      = 3000
  referenced_security_group_id = aws_security_group.alb.id
}

resource "aws_eip" "nat" {
  domain = "vpc"
}

# connectivity_type is ForceNew; it must round-trip through DescribeNatGateways
# or the provider plans destroy+create every time.
resource "aws_nat_gateway" "this" {
  allocation_id     = aws_eip.nat.id
  subnet_id         = aws_subnet.a.id
  connectivity_type = "public"
}

resource "aws_cloudwatch_log_group" "this" {
  name = "/fidelity/app"
  tags = {
    Name = "fidelity"
    env  = "ci"
  }
}

resource "aws_dynamodb_table" "this" {
  name         = "fidelity-table"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "PK"

  attribute {
    name = "PK"
    type = "S"
  }

  tags = {
    Name = "fidelity"
  }
}

resource "aws_ecr_repository" "this" {
  name = "fidelity-repo"
  tags = {
    Name = "fidelity"
  }
}

# Task-def tags are read by the provider via DescribeTaskDefinition
# --include TAGS (response top-level tags); a simple container def keeps the
# ForceNew containerDefinitions hash stable so only the tag path is exercised.
resource "aws_ecs_task_definition" "this" {
  family                   = "fidelity-control-plane"
  network_mode             = "awsvpc"
  requires_compatibilities = ["FARGATE"]
  cpu                      = "256"
  memory                   = "512"
  container_definitions = jsonencode([{
    name      = "app"
    image     = "nginx"
    essential = true
  }])
  tags = {
    Name            = "fidelity"
    "edd:component" = "ecs-dev-desktop"
  }
}

resource "aws_acm_certificate" "this" {
  domain_name       = "fidelity.example.test"
  validation_method = "DNS"
}

# ALB created without minimum_load_balancer_capacity: DescribeCapacityReservation
# must omit the attribute (not report capacity_units=0, which drifts to null).
resource "aws_lb" "this" {
  name               = "fidelity-alb"
  internal           = true
  load_balancer_type = "application"
  subnets            = [aws_subnet.a.id, aws_subnet.b.id]
  security_groups    = [aws_security_group.alb.id]
}

resource "aws_lb_target_group" "this" {
  name     = "fidelity-tg"
  port     = 80
  protocol = "HTTP"
  vpc_id   = aws_vpc.main.id
}

# HTTPS listener with an ACM cert: the cert ARN must round-trip through
# DescribeListeners or the listener-to-cert linkage is lost.
resource "aws_lb_listener" "https" {
  load_balancer_arn = aws_lb.this.arn
  port              = 443
  protocol          = "HTTPS"
  certificate_arn   = aws_acm_certificate.this.arn

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.this.arn
  }
}

output "nat_gateway_id" {
  value = aws_nat_gateway.this.id
}

output "listener_certificate_arn" {
  value = aws_lb_listener.https.certificate_arn
}
