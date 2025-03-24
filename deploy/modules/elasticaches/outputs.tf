output "providers" {
  value = {
    id      = aws_elasticache_replication_group.cache["providers"].id
    arn     = aws_elasticache_replication_group.cache["providers"].arn
    address = aws_elasticache_replication_group.cache["providers"].configuration_endpoint_address
    port    = aws_elasticache_replication_group.cache["providers"].port
  }
}

output "indexes" {
  value = {
    id      = aws_elasticache_replication_group.cache["indexes"].id
    arn     = aws_elasticache_replication_group.cache["indexes"].arn
    address = aws_elasticache_replication_group.cache["indexes"].configuration_endpoint_address
    port    = aws_elasticache_replication_group.cache["indexes"].port
  }
}

output "claims" {
  value = {
    id      = aws_elasticache_replication_group.cache["claims"].id
    arn     = aws_elasticache_replication_group.cache["claims"].arn
    address = aws_elasticache_replication_group.cache["claims"].configuration_endpoint_address
    port    = aws_elasticache_replication_group.cache["claims"].port
  }
}

output "iam_user" {
  value = {
    arn     = aws_elasticache_user.cache_iam_user.arn
    user_id = aws_elasticache_user.cache_iam_user.user_id
  }
}

output "security_group_id" {
  value = aws_security_group.cache_security_group.id
}
