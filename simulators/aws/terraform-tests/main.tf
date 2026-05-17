terraform {
  required_providers {
    aws = {
      source = "hashicorp/aws"
    }
  }
}

provider "aws" {
  region                      = "us-east-1"
  access_key                  = "test"
  secret_key                  = "test"
  skip_credentials_validation = true
  skip_metadata_api_check     = true
  skip_requesting_account_id  = true

  # Force path-style addressing for S3 so the bucket name appears in the
  # URL path (https://<endpoint>/<bucket>/...) instead of the subdomain
  # (https://<bucket>.<endpoint>/...). The sim mounts S3 routes under
  # /s3/{bucket} which only the path-style form reaches.
  s3_use_path_style = true

  endpoints {
    ecs              = var.endpoint
    sts              = var.endpoint
    ecr              = var.endpoint
    servicediscovery = var.endpoint
    cloudfront       = var.endpoint
    acm              = var.endpoint
    route53          = var.endpoint
    wafv2            = var.endpoint
    amplify          = var.endpoint
    iam              = var.endpoint
    s3               = "${var.endpoint}/s3"
    dynamodb         = var.endpoint
    kms              = var.endpoint
    secretsmanager   = var.endpoint
    ssm              = var.endpoint
  }
}

data "aws_caller_identity" "current" {}

resource "aws_ecs_cluster" "main" {
  name = "tf-test-cluster"
}

# Exercise the pull-through-cache APIs added to the simulator in
# BUG-696's fix. Terraform's aws_ecr_pull_through_cache_rule resource
# wraps the same CreatePullThroughCacheRule / DescribePullThroughCacheRules
# / DeletePullThroughCacheRule endpoints the SDK + CLI tests cover.
resource "aws_ecr_pull_through_cache_rule" "docker_hub" {
  ecr_repository_prefix = "tf-docker-hub"
  upstream_registry_url = "registry-1.docker.io"
}

# Exercise the Cloud Map namespace + service APIs that BUG-701's fix
# depends on. Creating the namespace in real AWS also creates an R53
# hosted zone and the matching Docker user-defined network in the
# simulator; the service configures the DNS record type used by
# per-hostname A-record services sockerless creates at runtime.
resource "aws_service_discovery_private_dns_namespace" "tf_svc_net" {
  name = "tf-svc-net.local"
  vpc  = "vpc-sim"
}

resource "aws_service_discovery_service" "tf_svc" {
  name = "tf-svc"

  dns_config {
    namespace_id   = aws_service_discovery_private_dns_namespace.tf_svc_net.id
    routing_policy = "MULTIVALUE"

    dns_records {
      ttl  = 10
      type = "A"
    }
  }
}

# Phase 159 — Exercise the CloudFront REST + XML wire on the simulator.
# Hits POST /2020-05-31/distribution + GET /2020-05-31/distribution/{id} +
# PUT /2020-05-31/distribution/{id}/config (Terraform sets Enabled=false
# automatically before destroy because the simulator enforces the real
# AWS "DistributionNotDisabled" precondition).
resource "aws_cloudfront_origin_access_control" "tf_oac" {
  name                              = "tf-oac"
  description                       = "tf-test"
  origin_access_control_origin_type = "s3"
  signing_behavior                  = "always"
  signing_protocol                  = "sigv4"
}

resource "aws_cloudfront_cache_policy" "tf_cp" {
  name        = "tf-cache-policy"
  comment     = "tf-test cache policy"
  default_ttl = 86400
  max_ttl     = 31536000
  min_ttl     = 1

  parameters_in_cache_key_and_forwarded_to_origin {
    enable_accept_encoding_gzip   = true
    enable_accept_encoding_brotli = true

    headers_config {
      header_behavior = "none"
    }
    cookies_config {
      cookie_behavior = "none"
    }
    query_strings_config {
      query_string_behavior = "none"
    }
  }
}

resource "aws_cloudfront_origin_request_policy" "tf_orp" {
  name    = "tf-origin-request-policy"
  comment = "tf-test origin request policy"

  headers_config {
    header_behavior = "none"
  }
  cookies_config {
    cookie_behavior = "none"
  }
  query_strings_config {
    query_string_behavior = "none"
  }
}

resource "aws_iam_service_linked_role" "tf_slr_cloudfront" {
  aws_service_name = "cloudfront.amazonaws.com"
  custom_suffix    = "tftest"
  description      = "tf-test CloudFront SLR"
}

resource "aws_iam_openid_connect_provider" "tf_oidc" {
  url             = "https://oidc.eks.us-east-1.amazonaws.com/id/TFTESTOIDC"
  client_id_list  = ["sts.amazonaws.com"]
  thumbprint_list = ["9e99a48a9960b14926bb7f3b02e22da2b0ab7280"]
}

resource "aws_amplify_app" "tf_amplify" {
  name        = "tf-amplify"
  description = "tf-test Amplify app"
  platform    = "WEB"

  environment_variables = {
    ENV = "test"
  }

  enable_branch_auto_build = true
  enable_basic_auth        = false
}

resource "aws_amplify_branch" "tf_amplify_main" {
  app_id      = aws_amplify_app.tf_amplify.id
  branch_name = "main"
  framework   = "Next.js - SSR"
  stage       = "PRODUCTION"
}

resource "aws_amplify_webhook" "tf_amplify_hook" {
  app_id      = aws_amplify_app.tf_amplify.id
  branch_name = aws_amplify_branch.tf_amplify_main.branch_name
  description = "tf-test webhook"
}

resource "aws_amplify_backend_environment" "tf_amplify_be" {
  app_id           = aws_amplify_app.tf_amplify.id
  environment_name = "staging"
  stack_name       = "amplify-staging-stack"
}

resource "aws_amplify_domain_association" "tf_amplify_domain" {
  app_id      = aws_amplify_app.tf_amplify.id
  domain_name = "tf-amplify.example.com"

  sub_domain {
    branch_name = aws_amplify_branch.tf_amplify_main.branch_name
    prefix      = "www"
  }

  sub_domain {
    branch_name = aws_amplify_branch.tf_amplify_main.branch_name
    prefix      = ""
  }
}

resource "aws_wafv2_ip_set" "tf_ipset" {
  name               = "tf-ipset"
  description        = "tf-test IP allowlist"
  scope              = "CLOUDFRONT"
  ip_address_version = "IPV4"
  addresses          = ["203.0.113.0/24", "198.51.100.10/32"]
}

resource "aws_wafv2_web_acl" "tf_acl" {
  name        = "tf-acl"
  description = "tf-test WebACL"
  scope       = "CLOUDFRONT"

  default_action {
    allow {}
  }

  visibility_config {
    cloudwatch_metrics_enabled = true
    metric_name                = "tf-acl-metric"
    sampled_requests_enabled   = true
  }

  rule {
    name     = "block-ipset"
    priority = 1

    action {
      block {}
    }

    statement {
      ip_set_reference_statement {
        arn = aws_wafv2_ip_set.tf_ipset.arn
      }
    }

    visibility_config {
      cloudwatch_metrics_enabled = true
      metric_name                = "tf-acl-block"
      sampled_requests_enabled   = true
    }
  }
}

resource "aws_wafv2_web_acl_association" "tf_assoc" {
  resource_arn = aws_cloudfront_distribution.tf_dist.arn
  web_acl_arn  = aws_wafv2_web_acl.tf_acl.arn
}

resource "aws_route53_zone" "tf_zone" {
  name    = "tf-route53.local"
  comment = "tf-test zone"
}

# A-record + ALIAS record. ALIAS targets the CloudFront distribution
# created below by reference; this exercises the cross-resource flow
# that real production stacks use (Route 53 ALIAS → CloudFront).
resource "aws_route53_record" "tf_a" {
  zone_id = aws_route53_zone.tf_zone.zone_id
  name    = "api.tf-route53.local"
  type    = "A"
  ttl     = 300
  records = ["203.0.113.42"]
}

resource "aws_route53_record" "tf_alias" {
  zone_id = aws_route53_zone.tf_zone.zone_id
  name    = "cdn.tf-route53.local"
  type    = "A"

  alias {
    name                   = aws_cloudfront_distribution.tf_dist.domain_name
    zone_id                = aws_cloudfront_distribution.tf_dist.hosted_zone_id
    evaluate_target_health = false
  }
}

resource "aws_acm_certificate" "tf_cert" {
  domain_name               = "tf-cert.example.com"
  subject_alternative_names = ["www.tf-cert.example.com"]
  validation_method         = "DNS"

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_cloudfront_function" "tf_fn" {
  name    = "tf-fn"
  runtime = "cloudfront-js-2.0"
  comment = "tf-test function"
  publish = true
  code    = <<-EOF
    function handler(event) {
      return event.request;
    }
  EOF
}

resource "aws_cloudfront_public_key" "tf_pk" {
  name        = "tf-pk"
  comment     = "tf-test public key"
  encoded_key = <<-EOF
    -----BEGIN PUBLIC KEY-----
    dGVzdC1rZXktYnl0ZXMtZm9yLXNpbXVsYXRvcg==
    -----END PUBLIC KEY-----
  EOF
}

resource "aws_cloudfront_key_group" "tf_kg" {
  name    = "tf-kg"
  comment = "tf-test key group"
  items   = [aws_cloudfront_public_key.tf_pk.id]
}

resource "aws_cloudfront_response_headers_policy" "tf_rhp" {
  name    = "tf-response-headers-policy"
  comment = "tf-test response headers policy"

  security_headers_config {
    content_type_options {
      override = true
    }
    frame_options {
      override     = true
      frame_option = "DENY"
    }
  }
}

resource "aws_cloudfront_distribution" "tf_dist" {
  enabled         = false # let terraform destroy without an explicit disable step
  is_ipv6_enabled = true
  comment         = "tf-test cloudfront"
  price_class     = "PriceClass_100"

  origin {
    domain_name              = "tf-origin.example.com"
    origin_id                = "tf-origin"
    origin_access_control_id = aws_cloudfront_origin_access_control.tf_oac.id

    custom_origin_config {
      http_port                = 80
      https_port               = 443
      origin_protocol_policy   = "https-only"
      origin_ssl_protocols     = ["TLSv1.2"]
      origin_read_timeout      = 30
      origin_keepalive_timeout = 5
    }
  }

  default_cache_behavior {
    target_origin_id       = "tf-origin"
    viewer_protocol_policy = "redirect-to-https"
    allowed_methods        = ["GET", "HEAD"]
    cached_methods         = ["GET", "HEAD"]

    forwarded_values {
      query_string = false
      cookies {
        forward = "none"
      }
    }

    min_ttl     = 0
    default_ttl = 0
    max_ttl     = 0
  }

  restrictions {
    geo_restriction {
      restriction_type = "none"
    }
  }

  viewer_certificate {
    cloudfront_default_certificate = true
  }
}

# ---------- Runner foundation (S3 / DynamoDB / KMS / SecretsManager / SSM) ----------

# S3 bucket — runner backends stash workflow artifacts + terraform state.
resource "aws_s3_bucket" "tf_bucket" {
  bucket        = "tf-test-runner-bucket"
  force_destroy = true
}

# DynamoDB table — runner state-locking + per-pod registries.
resource "aws_dynamodb_table" "tf_table" {
  name         = "tf-test-table"
  hash_key     = "LockID"
  billing_mode = "PAY_PER_REQUEST"

  attribute {
    name = "LockID"
    type = "S"
  }
}

# KMS key — encrypts SecretsManager secrets and S3 objects.
resource "aws_kms_key" "tf_kms" {
  description             = "tf-test runner KMS key"
  deletion_window_in_days = 7
}

resource "aws_kms_alias" "tf_kms_alias" {
  name          = "alias/tf-test-runner"
  target_key_id = aws_kms_key.tf_kms.key_id
}

# Secrets Manager secret + version — runner credentials.
resource "aws_secretsmanager_secret" "tf_secret" {
  name                    = "tf-test-runner-secret"
  recovery_window_in_days = 0
}

resource "aws_secretsmanager_secret_version" "tf_secret_v1" {
  secret_id     = aws_secretsmanager_secret.tf_secret.id
  secret_string = "tf-test-runner-secret-payload"
}

# SSM Parameter — runtime config injection (ECS task-def envFrom).
resource "aws_ssm_parameter" "tf_param" {
  name  = "/tf-test/runner/config"
  type  = "String"
  value = "tf-test-config-value"
}

# Phase 159.10 end-to-end stack outputs — verify the production-shape
# cross-resource links converge after apply. apply_test.go asserts that
# WAF.resource_arn == CloudFront.arn, Route 53 ALIAS target == CloudFront
# domain_name, and the ACM cert ARN region is us-east-1 (CloudFront pin).
output "cloudfront_arn" {
  value = aws_cloudfront_distribution.tf_dist.arn
}
output "cloudfront_domain_name" {
  value = aws_cloudfront_distribution.tf_dist.domain_name
}
output "cloudfront_hosted_zone_id" {
  value = aws_cloudfront_distribution.tf_dist.hosted_zone_id
}
output "acm_certificate_arn" {
  value = aws_acm_certificate.tf_cert.arn
}
output "wafv2_assoc_resource_arn" {
  value = aws_wafv2_web_acl_association.tf_assoc.resource_arn
}
output "wafv2_assoc_webacl_arn" {
  value = aws_wafv2_web_acl_association.tf_assoc.web_acl_arn
}
output "route53_alias_target_name" {
  value = aws_route53_record.tf_alias.alias[0].name
}
output "route53_alias_target_zone_id" {
  value = aws_route53_record.tf_alias.alias[0].zone_id
}
output "amplify_app_arn" {
  value = aws_amplify_app.tf_amplify.arn
}
output "iam_slr_arn" {
  value = aws_iam_service_linked_role.tf_slr_cloudfront.arn
}
output "s3_bucket_arn" {
  value = aws_s3_bucket.tf_bucket.arn
}
output "dynamodb_table_arn" {
  value = aws_dynamodb_table.tf_table.arn
}
output "kms_key_arn" {
  value = aws_kms_key.tf_kms.arn
}
output "kms_alias_arn" {
  value = aws_kms_alias.tf_kms_alias.arn
}
output "secretsmanager_secret_arn" {
  value = aws_secretsmanager_secret.tf_secret.arn
}
output "ssm_parameter_arn" {
  value = aws_ssm_parameter.tf_param.arn
}
