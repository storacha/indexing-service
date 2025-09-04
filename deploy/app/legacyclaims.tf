locals {
    legacy_claims_table_name = "${terraform.workspace == "prod" ? "prod" : "staging"}-content-claims-claims-v1"
    legacy_claims_table_region = "${terraform.workspace == "prod" ? "us-west-2" : "us-east-2"}"
    legacy_claims_bucket_name = "${terraform.workspace == "prod" ? "prod-content-claims-bucket-claimsv1bucketefd46802-1mqz6d8o7xw8" : "staging-content-claims-buc-claimsv1bucketefd46802-1xx2brszve6t3"}"
    legacy_block_index_table_name = "${terraform.workspace == "prod" ? "prod" : "staging"}-ep-v1-blocks-cars-position"
    legacy_allocations_table_name = "${terraform.workspace == "prod" ? "prod" : "staging"}-w3infra-allocation"
    legacy_allocations_table_region = "${terraform.workspace == "prod" ? "us-west-2" : "us-east-2"}"
    legacy_store_table_name = "${terraform.workspace == "prod" ? "prod" : "staging"}-w3infra-store"
    legacy_store_table_region = "${terraform.workspace == "prod" ? "us-west-2" : "us-east-2"}"
    legacy_blob_registry_table_name = "${terraform.workspace == "prod" ? "prod" : "staging"}-w3infra-blob-registry"
    legacy_blob_registry_table_region = "${terraform.workspace == "prod" ? "us-west-2" : "us-east-2"}"
}

provider "aws" {
  alias = "legacy_claims"
  region = local.legacy_claims_table_region
}

data "aws_s3_bucket" "legacy_claims_bucket" {
  provider = aws.legacy_claims
  bucket = local.legacy_claims_bucket_name
}

data "aws_dynamodb_table" "legacy_claims_table" {
  provider = aws.legacy_claims
  name = local.legacy_claims_table_name
}

provider "aws" {
  alias = "block_index"
  region = "us-west-2"
}

data "aws_dynamodb_table" "legacy_block_index_table" {
  provider = aws.block_index
  name = local.legacy_block_index_table_name
}


provider "aws" {
  alias = "allocations"
  region = local.legacy_allocations_table_region
}


data "aws_dynamodb_table" "legacy_allocations_table" {
  provider = aws.allocations
  name = local.legacy_allocations_table_name
}

provider "aws" {
  alias = "store"
  region = local.legacy_store_table_region
}

data "aws_dynamodb_table" "legacy_store_table" {
  provider = aws.store
  name = local.legacy_store_table_name
}

provider "aws" {
  alias = "blob_registry"
  region = local.legacy_blob_registry_table_region
}

data "aws_dynamodb_table" "legacy_blob_registry_table" {
  provider = aws.blob_registry
  name = local.legacy_blob_registry_table_name
}

data "aws_iam_policy_document" "task_legacy_dynamodb_query_document" {
  statement {
    actions = [
      "dynamodb:Query",
    ]
    resources = [
      data.aws_dynamodb_table.legacy_claims_table.arn,
      data.aws_dynamodb_table.legacy_block_index_table.arn,
      data.aws_dynamodb_table.legacy_allocations_table.arn,
      "${data.aws_dynamodb_table.legacy_allocations_table.arn}/index/*",
      "${data.aws_dynamodb_table.legacy_store_table.arn}/index/cid",
      "${data.aws_dynamodb_table.legacy_blob_registry_table.arn}/index/digest",
    ]
  }
}

resource "aws_iam_policy" "task_legacy_dynamodb_query" {
  name        = "${terraform.workspace}-${var.app}-task-legacy-dynamodb-query"
  description = "This policy will be used by the ECS task to query data from legacy DynamoDB tables"
  policy      = data.aws_iam_policy_document.task_legacy_dynamodb_query_document.json
}

resource "aws_iam_role_policy_attachment" "task_legacy_dynamodb_query" {
  role       = module.app.deployment.task_role.name
  policy_arn = aws_iam_policy.task_legacy_dynamodb_query.arn
}

data "aws_iam_policy_document" "task_legacy_s3_get_document" {
  statement {
    actions = [
      "s3:GetObject",
    ]
    resources = [
      "${data.aws_s3_bucket.legacy_claims_bucket.arn}/*"
    ]
  }
  statement {
    actions = [
      "s3:ListBucket","s3:GetBucketLocation"
    ]
    resources = [
      data.aws_s3_bucket.legacy_claims_bucket.arn
    ]
  }
}

resource "aws_iam_policy" "task_legacy_s3_get" {
  name        = "${terraform.workspace}-${var.app}-task-legacy-s3-get"
  description = "This policy will be used by the ECS task to get objects from legacy S3 buckets"
  policy      = data.aws_iam_policy_document.task_legacy_s3_get_document.json
}

resource "aws_iam_role_policy_attachment" "task_legacy_s3_get" {
  role       = module.app.deployment.task_role.name
  policy_arn = aws_iam_policy.task_legacy_s3_get.arn
}
