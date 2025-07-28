terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 5.86.0"
    }
    archive = {
      source = "hashicorp/archive"
    }
  }
  backend "s3" {
    bucket = "storacha-terraform-state"
    key = "storacha/${var.app}/terraform.tfstate"
    region = "us-west-2"
    encrypt = true
  }
}

provider "aws" {
  allowed_account_ids = [var.allowed_account_id]
  region = var.region
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

# CloudFront is a global service. Certs must be created in us-east-1, where the core ACM infra lives
provider "aws" {
  region = "us-east-1"
  alias = "acm"
}



module "app" {
  source = "github.com/storacha/storoku//app?ref=v0.4.2"
  private_key = var.private_key
  httpport = 8080
  principal_mapping = var.principal_mapping
  did = var.did
  app = var.app
  appState = var.app
  write_to_container = false
  environment = terraform.workspace
  network = var.network
  # if there are any env vars you want available only to your container
  # in the vpc as opposed to set in the dockerfile, enter them here
  # NOTE: do not put sensitive data in env-vars. use secrets
  deployment_env_vars = []
  image_tag = var.image_tag
  create_db = false
  # enter secret values your app will use here -- these will be available
  # as env vars in the container at runtime
  secrets = { 
  }
  # enter any sqs queues you want to create here
  queues = [
    {
      name = "provider-caching"
      fifo = false
      message_retention_seconds = 86400
    },
  ]
  caches = ["providers","no-providers","indexes","claims",]
  topics = []
  tables = [
    {
      name = "metadata"
      attributes = [
        {
          name = "provider"
          type = "S"
        },
        {
          name = "contextID"
          type = "B"
        },
      ]
      hash_key = "provider"
      range_key ="contextID"
    },
    {
      name = "chunk-links"
      attributes = [
        {
          name = "provider"
          type = "S"
        },
        {
          name = "contextID"
          type = "B"
        },
      ]
      hash_key = "provider"
      range_key ="contextID"
    },
  ]
  buckets = [
    {
      name = "provider-caching-bucket"
      public = false
      object_expiration_days = 14
    },
    {
      name = "ipni-store-bucket"
      public = true
    },
    {
      name = "notifier-head-bucket"
      public = false
    },
    {
      name = "claim-store-bucket"
      public = false
    },
  ]
  providers = {
    aws = aws
    aws.acm = aws.acm
  }
  env_files = var.env_files
  domain_base = var.domain_base
}
