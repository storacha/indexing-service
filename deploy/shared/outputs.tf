output "primary_zone" {
  value = aws_route53_zone.primary
}

output "dev_vpc" {
  value = {
    id = module.dev_vpc[0].id
    cidr_block = module.dev_vpc[0].cidr_block
    subnet_ids = module.dev_vpc[0].subnet_ids
  }
}

output "dev_caches" {
  value = {
    providers = module.dev_caches[0].providers
    no_providers = module.dev_caches[0].no_providers
    indexes = module.dev_caches[0].indexes
    claims = module.dev_caches[0].claims
    iam_user = module.dev_caches[0].iam_user
    security_group_id = module.dev_caches[0].security_group_id
  }
}
