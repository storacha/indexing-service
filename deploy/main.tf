terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 5.32.0"
    }
    archive = {
      source = "hashicorp/archive"
    }
  }
  backend "s3" {
    bucket = "storacha-terraform-state"
    key    = "storacha/${var.app}/terraform.tfstate"
    region = "us-west-2"
  }
}

provider "aws" {
  region              = var.region
  allowed_account_ids = ["505595374361"]
  default_tags {
    
    tags = {
      "Environment" = terraform.workspace
      "ManagedBy"   = "OpenTofu"
      Owner         = "storacha"
      Team          = "Storacha Engineer"
      Organization  = "Storacha"
    }
  }
}
