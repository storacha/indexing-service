locals {
  # Only prod and staging get their own VPC. All other envs will share the dev VPC
  should_create_vpc = terraform.workspace == "prod" || terraform.workspace == "staging"
}

module "vpc" {
  count = local.should_create_vpc ? 1 : 0

  source = "../../../modules/vpc"

  app = var.app
  environment = terraform.workspace
}
