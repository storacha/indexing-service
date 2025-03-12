locals {
  # Only prod and staging get their own caches. All other envs will share the dev caches
  should_create_shared_caches = terraform.workspace != "prod" && terraform.workspace != "staging"
}

module "caches" {
  count = local.should_create_shared_caches ? 1 : 0

  source = "../modules/elasticaches"

  app = var.app
  environment = "dev"
  
  cache_limits = {
    data_storage_GB = 1
    ecpu_per_second = 1000
  }
  
  vpc = {
    id = module.vpc[0].id
    cidr_block = module.vpc[0].cidr_block
    private_subnet_ids = module.vpc[0].subnet_ids.private
  }
}