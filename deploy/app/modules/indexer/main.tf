terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 5.86.0"
      configuration_aliases = [ aws.legacy_claims, aws.block_index ]
    }
  }
}

# access state for shared resources (primary DNS zone, dev VPC and dev caches)
data "terraform_remote_state" "shared" {
  backend = "s3"
  config = {
    bucket = "storacha-terraform-state"
    key    = "storacha/indexing-service/shared.tfstate"
    region = "us-west-2"
  }
}

data "aws_region" "current" {}
