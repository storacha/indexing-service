locals {
  caches = toset(["providers","indexes","claims"])
}
resource "aws_kms_key" "cache_key" {
  description         = "KMS CMK for ${terraform.workspace} ${var.app}"
  enable_key_rotation = true
}

resource "aws_elasticache_serverless_cache" "cache" {
  for_each = local.caches
  
  engine = "valkey"
  name = "${terraform.workspace}-${var.app}-${each.key}-cache"
  cache_usage_limits {
    data_storage {
      maximum = terraform.workspace == "prod" ? 10 : 1
      unit = "GB"
    }
    ecpu_per_second {
      maximum = terraform.workspace == "prod" ? 10000 : 1000
    }
  }
  daily_snapshot_time  = "02:00"
  description          = "${terraform.workspace} ${var.app} ${each.key} serverless cluster"
  kms_key_id           = aws_kms_key.cache_key.arn
  major_engine_version = "7"
  security_group_ids   = [aws_security_group.cache_security_group.id]

  snapshot_retention_limit = 7
  subnet_ids               = aws_subnet.vpc_private_subnet[*].id

  user_group_id = aws_elasticache_user_group.cache_user_group.user_group_id
}

resource "aws_elasticache_user_group" "cache_user_group" {
  engine = "valkey"
  user_group_id = "${terraform.workspace}-${var.app}-valkey"

  user_ids = [
    aws_elasticache_user.cache_default_user.id, 
    aws_elasticache_user.cache_iam_user.id
  ]

  lifecycle {
    ignore_changes = [user_ids]
  }
}

resource "aws_elasticache_user" "cache_default_user" {
  user_id       = "${terraform.workspace}-${var.app}-default-disabled"
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
  user_id       = "${terraform.workspace}-${var.app}-iam-user"
  user_name     = "${terraform.workspace}-${var.app}-iam-user"
  access_string = "on ~* +@all"
  authentication_mode {
    type = "iam"
  }
  engine = "valkey"
}

resource "aws_security_group" "cache_security_group" {

  name        = "${terraform.workspace}-${var.app}-cache-security-group"
  description = "Security group for VPC access to redis"
  vpc_id      = aws_vpc.vpc.id
  ingress {
    cidr_blocks = [aws_vpc.vpc.cidr_block]
    description = "Redis"
    from_port = 6379
    to_port = 6380
    protocol = "tcp"
  }
}