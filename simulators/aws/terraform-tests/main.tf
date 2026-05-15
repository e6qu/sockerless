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

  endpoints {
    ecs              = var.endpoint
    sts              = var.endpoint
    ecr              = var.endpoint
    servicediscovery = var.endpoint
    cloudfront       = var.endpoint
    acm              = var.endpoint
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
