terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 5.86.0"
    }
  }
}

locals {
  caches = toset(["providers", "indexes", "claims"])
}

resource "aws_kms_key" "cache_key" {
  description         = "KMS CMK for ${var.environment} ${var.app} caches"
  enable_key_rotation = true
}

resource "aws_elasticache_replication_group" "cache" {
  for_each = local.caches

  replication_group_id = "${var.environment}-${var.app}-${each.key}-cache"
  description          = "${var.environment} ${var.app} ${each.key} cache cluster"

  engine               = "valkey"
  engine_version       = "8.0"
  node_type            = var.node_type
  port                 = 6379
  parameter_group_name = aws_elasticache_parameter_group.cluster_enabled.name

  num_node_groups         = 1 # 1 shard
  replicas_per_node_group = 2 # 2 read replicas per shard

  multi_az_enabled           = true
  automatic_failover_enabled = true

  snapshot_window          = "02:00-05:00"
  snapshot_retention_limit = 7

  subnet_group_name = aws_elasticache_subnet_group.cache_subnet_group.name

  transit_encryption_enabled = true
  transit_encryption_mode    = "required"
  at_rest_encryption_enabled = true
  kms_key_id                 = aws_kms_key.cache_key.arn
  security_group_ids         = [aws_security_group.cache_security_group.id]
  user_group_ids             = [aws_elasticache_user_group.cache_user_group.user_group_id]

  apply_immediately = true
}

resource "aws_elasticache_parameter_group" "cluster_enabled" {
  name   = "${var.environment}-${var.app}-valkey8-params"
  family = "valkey8"

  parameter {
    name  = "cluster-enabled"
    value = "yes"
  }
  parameter {
    name  = "maxmemory-policy"
    value = "volatile-lfu"
  }

  parameter {
    name  = "maxmemory-samples"
    value = "10"
  }
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
    to_port = 6379
    protocol = "tcp"
  }
}

resource "aws_elasticache_subnet_group" "cache_subnet_group" {
  name       = "${var.environment}-${var.app}-cache-subnet-group"
  subnet_ids = var.vpc.private_subnet_ids
}
