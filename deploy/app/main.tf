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

module "us-west" {
  source = "./modules/indexer"
  
  app = var.app
  private_key = var.private_key
  did = var.did
  honeycomb_api_key = var.honeycomb_api_key
  principal_mapping = var.principal_mapping
  legacy_data_bucket_url = var.legacy_data_bucket_url

  providers = {
    aws = aws.us-west
  }
}

module "us-east" {
  source = "./modules/indexer"

  app = var.app
  private_key = var.private_key
  did = var.did
  honeycomb_api_key = var.honeycomb_api_key
  principal_mapping = var.principal_mapping
  legacy_data_bucket_url = var.legacy_data_bucket_url

  providers = {
    aws = aws.us-east
  }
}

module "europe" {
  source = "./modules/indexer"

  app = var.app
  private_key = var.private_key
  did = var.did
  honeycomb_api_key = var.honeycomb_api_key
  principal_mapping = var.principal_mapping
  legacy_data_bucket_url = var.legacy_data_bucket_url

  providers = {
    aws = aws.europe
  }
}
