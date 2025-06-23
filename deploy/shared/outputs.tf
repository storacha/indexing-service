output "route53_zones" {
  value = module.shared.route53_zones
}

output "dev_vpc" {
  value = module.shared.dev_vpc
}

output "dev_caches" {
  value = module.shared.dev_caches
}

output "dev_databases" {
  value = module.shared.dev_databases
}

output "dev_kms" {
  value = module.shared.dev_kms
}