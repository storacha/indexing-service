locals {
  # Only prod and staging get their own VPC. All other envs will share the dev VPC
  should_create_shared_vpc = !local.is_production && !local.is_staging
}

module "dev_vpc" {
  count = local.should_create_shared_vpc ? 1 : 0

  source = "../modules/vpc"

  app = var.app
  environment = "dev"
}
