locals {
  caches = ["providers","indexes","claims"]
}
resource "aws_kms_key" "cache_key" {
  description         = "KMS CMK for ${terraform.workspace} ${var.app}"
  enable_key_rotation = true
}

resource "aws_elasticache_serverless_cache" "cache" {
  for_each = local.caches
  
  engine = "REDIS"
  name = "${terraform.workspace}-${var.app}-${each.key}-cache"
  cache_usage_limits {
    data_storage {
      maximum = terraform.workspace == "prod" ? 10 : 1
      unit = "GB"
    }
    ecpu_per_second {
      maximum = terraform.workspace == "prod" ? 5000 : 500
    }
  }
  daily_snapshot_time  = "2:00"
  description          = "${terraform.workspace} ${var.app} ${each.key} serverless cluster"
  kms_key_id           = aws_kms_key.cache_key.arn
  major_engine_version = "7"
  security_group_ids   = [aws_security_group.cache_security_group.id]

  snapshot_retention_limit = 7
  subnet_ids               = aws_subnet.vpc_private_subnet[*].id

  user_group_id = aws_elasticache_user_group.cache_user_group.user_group_id
}

resource "aws_elasticache_user_group" "cache_user_group" {
  engine = "REDIS"
  user_group_id = "${terraform.workspace}-${var.app}-redis"

  user_ids = [
    "${terraform.workspace}-${var.app}-default-disabled", 
    "${terraform.workspace}-${var.app}-iam-user"
  ]

  lifecycle {
    ignore_changes = [user_ids]
  }
}

resource "aws_elasticache_user" "cache_default_user" {
  user_id       = "${terraform.workspace}-${var.app}-default-disabled"
  user_name     = "default"
  access_string = "off +get ~keys*"
  authentication_mode {
    type = "no-password-required"
  }
  engine = "REDIS"
}

resource "aws_elasticache_user" "cache_iam_user" {
  user_id       = "${terraform.workspace}-${var.app}-iam-user"
  user_name     = "iam-user"
  access_string = "on ~* +@all"
  authentication_mode {
    type = "iam"
  }
  engine = "REDIS"
}

resource "aws_security_group" "cache_security_group" {

  name        = "${terraform.workspace}-${var.app}-cache-security-group"
  description = "Security group for VPC access to redis"
  vpc_id      = module.vpc.vpc_id
}

resource "aws_security_group_rule" "cache_security_group_rule" {
  security_group_id = aws_security_group.cache_security_group.id
  type = "ingress"

  cidr_blocks = [aws_vpc.vpc.cidr_block]
  description = "Redis"
  from_port = 6379
  to_port = 6379
  protocol = "tcp"
}