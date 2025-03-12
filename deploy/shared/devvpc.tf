locals {
  # Only prod and staging get their own VPC. All other envs will share the dev VPC
  create_shared_vpc = terraform.workspace != "prod" && terraform.workspace != "staging"
}

module "vpc" {
  count = local.create_shared_vpc ? 1 : 0

  source = "../modules/vpc"

  app = var.app
  environment = "dev"
}
