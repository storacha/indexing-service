terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 5.86.0"
    }
  }
  backend "s3" {
    bucket = "storacha-terraform-state"
    key    = "storacha/indexing-service/shared.tfstate"
    region = "us-west-2"
  }
}

provider "aws" {
  allowed_account_ids = var.allowed_account_ids
  default_tags {
    tags = {
      "Environment" = terraform.workspace
      "ManagedBy"   = "OpenTofu"
      Owner         = "storacha"
      Team          = "Storacha Engineering"
      Organization  = "Storacha"
      Project       = "${var.app}"
    }
  }
}

removed {
  from = aws_route53_zone.primary
}

import {
  to = aws_route53_zone.hot
  id = "Z069841432CRU732HASNL"
}

resource "aws_route53_zone" "hot" {
  name = "${var.app}.storacha.network"
}

import {
  to = aws_route53_zone.warm
  id = "Z0845167J9GR7IUCNBTW"
}

resource "aws_route53_zone" "warm" {
  name = "${var.app}.warm.storacha.network"
}
