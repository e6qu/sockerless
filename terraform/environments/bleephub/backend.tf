terraform {
  backend "s3" {
    bucket  = "bleephub-terraform-state-729079515331"
    key     = "production/terraform.tfstate"
    region  = "eu-west-1"
    encrypt = true
  }
}
