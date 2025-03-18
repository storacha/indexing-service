locals {
  # Only do a multi-region deployment for prod
  # The special "provider" region allows keeping the original provider definition when removing resources in all
  # other regions (see https://opentofu.org/docs/language/providers/configuration/#referring-to-provider-instances)
  deployment_regions = terraform.workspace == "prod" ? concat(var.deployment_regions, ["provider"]) : [var.deployment_regions[0], "provider"]
}

provider "aws" {
  alias  = "by_region"
  for_each = toset(local.deployment_regions)

  allowed_account_ids = var.allowed_account_ids

  default_tags { 
    tags = {
      Environment  = terraform.workspace
      ManagedBy    = "OpenTofu"
      Owner        = "storacha"
      Team         = "Storacha Engineering"
      Organization = "Storacha"
      Project      = var.app
    }
  }
}
