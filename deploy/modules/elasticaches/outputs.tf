output "providers" {
  value = {
    arn = aws_elasticache_serverless_cache.cache["providers"].arn
    address = aws_elasticache_serverless_cache.cache["providers"].endpoint[0].address
    name = aws_elasticache_serverless_cache.cache["providers"].name
  }
}
output "no_providers" {
  value = {
    arn = aws_elasticache_serverless_cache.cache["no-providers"].arn
    address = aws_elasticache_serverless_cache.cache["no-providers"].endpoint[0].address
    name = aws_elasticache_serverless_cache.cache["no-providers"].name
  }
}

output "indexes" {
  value = {
    arn = aws_elasticache_serverless_cache.cache["indexes"].arn
    address = aws_elasticache_serverless_cache.cache["indexes"].endpoint[0].address
    name = aws_elasticache_serverless_cache.cache["indexes"].name
  }
}

output "claims" {
  value = {
    arn = aws_elasticache_serverless_cache.cache["claims"].arn
    address = aws_elasticache_serverless_cache.cache["claims"].endpoint[0].address
    name = aws_elasticache_serverless_cache.cache["claims"].name
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
