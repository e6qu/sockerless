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
