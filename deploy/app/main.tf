terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 5.86.0"
    }
    archive = {
      source = "hashicorp/archive"
    }
  }

  backend "s3" {
    bucket = "storacha-terraform-state"
    key    = "storacha/indexing-service/terraform.tfstate"
    region = "us-west-2"
  }
}

locals {
  # Only do a multi-region deployment for prod
  deployment_regions = terraform.workspace == "prod" ? var.deployment_regions : [var.deployment_regions[0]]
}

module "indexer_deployment" {
  for_each = var.deployment_regions

  source = "./modules/indexer"

  region = each.value

  app = var.app
  private_key = var.private_key
  did = var.did
  honeycomb_api_key = var.honeycomb_api_key
  principal_mapping = var.principal_mapping
  legacy_data_bucket_url = var.legacy_data_bucket_url
}
