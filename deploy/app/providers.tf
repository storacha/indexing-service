locals {
  # Only do a multi-region deployment for prod
  # The special "provider" region allows keeping the original provider definition when removing resources in all
  # other regions (see https://opentofu.org/docs/language/providers/configuration/#referring-to-provider-instances)
  extra_deployment_regions = terraform.workspace == "prod" ? concat(var.extra_deployment_regions, ["provider"]) : ["provider"]

  tags = {
      Environment  = terraform.workspace
      ManagedBy    = "OpenTofu"
      Owner        = "storacha"
      Team         = "Storacha Engineering"
      Organization = "Storacha"
      Project      = var.app
    }
}

provider "aws" {
  allowed_account_ids = var.allowed_account_ids

  default_tags {
    tags = local.tags
  }
}

provider "aws" {
  alias  = "extra_deployments"
  for_each = toset(local.extra_deployment_regions)

  allowed_account_ids = var.allowed_account_ids

  default_tags {
    tags = local.tags
  }
}
