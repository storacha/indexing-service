locals {
  # Only prod and staging get their own caches. All other envs will share the dev caches
  should_create_shared_caches = terraform.workspace != "prod" && terraform.workspace != "staging"
}

module "dev_caches" {
  count = local.should_create_shared_caches ? 1 : 0

  source = "../modules/elasticaches"

  app = var.app
  environment = "dev"
  node_type = "cache.t4g.micro" # 2 vCPUs, 0.5 GiB memory, 5 Gigabit network, $0.0128/hour
  
  vpc = {
    id = module.dev_vpc[0].id
    cidr_block = module.dev_vpc[0].cidr_block
    private_subnet_ids = module.dev_vpc[0].subnet_ids.private
  }
}