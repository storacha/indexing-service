locals {
  # Only prod and staging get their own caches. All other envs will share the dev caches
  should_create_caches = local.is_production || local.is_staging
}

module "caches" {
  count = local.should_create_caches ? 1 : 0

  source = "../modules/elasticaches"

  app = var.app
  environment = terraform.workspace

  # cache.r7g.large => 2 vCPUs, 13.07 GiB memory, 12.5 Gigabit network, $0.1752/hour
  # cache.t4g.micro => 2 vCPUs, 0.5 GiB memory, 5 Gigabit network, $0.0128/hour
  node_type = local.is_production ? "cache.r7g.large" : "cache.t4g.micro"
  
  vpc = {
    id = module.vpc[0].id
    cidr_block = module.vpc[0].cidr_block
    private_subnet_ids = module.vpc[0].subnet_ids.private
  }
}