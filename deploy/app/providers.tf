provider "aws" {
  alias  = "us-west"
  region = "us-west-2"
  allowed_account_ids = var.allowed_account_ids

  default_tags { 
    tags = local.common_tags
  }
}

provider "aws" {
  alias  = "us-east"
  region = "us-east-1"
  allowed_account_ids = var.allowed_account_ids

  default_tags { 
    tags = local.common_tags
  }
}

provider "aws" {
  alias  = "europe"
  region = "eu-central-1"
  allowed_account_ids = var.allowed_account_ids

  default_tags { 
    tags = local.common_tags
  }
}

locals {
  common_tags = {
      Environment  = terraform.workspace
      ManagedBy    = "OpenTofu"
      Owner        = "storacha"
      Team         = "Storacha Engineering"
      Organization = "Storacha"
      Project      = var.app
    }
}
