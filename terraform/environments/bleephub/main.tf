terraform {
  required_version = ">= 1.9"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.7"
    }
    tls = {
      source  = "hashicorp/tls"
      version = "~> 4.1"
    }
  }
}

provider "aws" {
  region = "eu-west-1"
}

resource "random_password" "initial_admin_token" {
  length  = 48
  special = false
}

# The Bleephub release registry is owned by this stack rather than by EDD.
# The repository was created before this root existed and is imported into this
# resource's state as part of the initial production reconciliation.
resource "aws_ecr_repository" "bleephub" {
  name                 = "bleephub"
  image_tag_mutability = "IMMUTABLE"

  image_scanning_configuration {
    scan_on_push = true
  }

  encryption_configuration {
    encryption_type = "AES256"
  }

  tags = {
    component   = "bleephub"
    environment = "production"
    managed-by  = "terraform"
    stack       = "bleephub"
  }
}

module "bleephub" {
  source = "../../modules/bleephub-ecs"

  name                           = "bleephub-prod"
  region                         = "eu-west-1"
  hosted_zone_id                 = "Z04335081VP1Y44RQQQDD"
  domain_name                    = "bleephub.e6qu.dev"
  container_image                = "${aws_ecr_repository.bleephub.repository_url}@sha256:25b79c533a706254277870a7d4a42c06ce7240cb23df4d27546a16d1788e8203"
  admin_token                    = random_password.initial_admin_token.result
  wake_listener_zip_path         = "../../../.build/bleephub-ecs/bleephub-wake.zip"
  github_oauth_client_id         = "Ov23liRTMUqMD0gm5QTA"
  github_oauth_client_secret_arn = "arn:aws:secretsmanager:eu-west-1:729079515331:secret:bleephub/github-oauth-client-secret-eWXCQV"

  tags = {
    environment = "production"
  }
}

output "url" { value = module.bleephub.service_url }
output "admin_url" { value = module.bleephub.admin_url }
output "ssh_host" { value = module.bleephub.ssh_host }
output "admin_token_secret_arn" { value = module.bleephub.admin_token_secret_arn }
output "api_gateway_id" { value = module.bleephub.api_gateway_id }
