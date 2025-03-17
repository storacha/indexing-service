locals {
  caches = toset(["providers", "indexes", "claims"])
}

resource "aws_kms_key" "cache_key" {
  description         = "KMS CMK for ${var.environment} ${var.app} caches"
  enable_key_rotation = true
}

resource "aws_elasticache_serverless_cache" "cache" {
  for_each = local.caches
  
  engine = "valkey"
  name = "${var.environment}-${var.app}-${each.key}-cache"

  cache_usage_limits {
    data_storage {
      maximum = var.cache_limits.data_storage_GB
      unit = "GB"
    }

    ecpu_per_second {
      maximum = var.cache_limits.ecpu_per_second
    }
  }

  daily_snapshot_time  = "02:00"
  description          = "${var.environment} ${var.app} ${each.key} cache serverless cluster"
  kms_key_id           = aws_kms_key.cache_key.arn
  major_engine_version = "7"
  security_group_ids   = [aws_security_group.cache_security_group.id]

  snapshot_retention_limit = 7
  subnet_ids               = var.vpc.private_subnet_ids

  user_group_id = aws_elasticache_user_group.cache_user_group.user_group_id
}

resource "aws_elasticache_user_group" "cache_user_group" {
  engine = "valkey"
  user_group_id = "${var.environment}-${var.app}-valkey"

  user_ids = [
    aws_elasticache_user.cache_default_user.id, 
    aws_elasticache_user.cache_iam_user.id
  ]

  lifecycle {
    ignore_changes = [user_ids]
  }
}

resource "aws_elasticache_user" "cache_default_user" {
  user_id       = "${var.environment}-${var.app}-default-disabled"
  user_name     = "default"
  access_string = "off ~keys* -@all +get"
  
  authentication_mode {
    type = "password"
    passwords = ["does not matter its disabled"]
  }
  
  lifecycle {
    ignore_changes = [authentication_mode]
  }
  
  engine = "valkey"
}

resource "aws_elasticache_user" "cache_iam_user" {
  user_id       = "${var.environment}-${var.app}-cache-iam-user"
  user_name     = "${var.environment}-${var.app}-cache-iam-user"
  access_string = "on ~* +@all"
  
  authentication_mode {
    type = "iam"
  }

  engine = "valkey"
}

resource "aws_security_group" "cache_security_group" {
  name        = "${var.environment}-${var.app}-cache-security-group"
  description = "Security group for VPC access to redis"
  vpc_id      = var.vpc.id
  
  ingress {
    cidr_blocks = [var.vpc.cidr_block]
    description = "valkey cluster"
    from_port = 6379
    to_port = 6380
    protocol = "tcp"
  }
}
