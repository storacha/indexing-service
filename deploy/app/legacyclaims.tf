variable "legacy_claims_table_name" {
  description = "The name of the DynamoDB table used by the legacy content-claims service"
  type = string
  default = ""
}

variable "legacy_claims_table_region" {
  description = "The region where the legacy content-claims DynamoDB table is provisioned"
  type = string
  default = ""
}

variable "legacy_claims_bucket_name" {
  description = "The name of the S3 bucket used by the legacy content-claims service"
  type = string
  default = ""
}

variable "legacy_block_index_table_name" {
  description = "The name of the legacy block index DynamoDB table"
  type = string
  default = ""
}

# the block index table is always deployed in us-west-2 for both prod and staging
variable "legacy_block_index_table_region" {
  description = "The region where the legacy block index DynamoDB table is provisioned"
  type = string
  default = "us-west-2"
}

locals {
    inferred_legacy_claims_table_name = var.legacy_claims_table_name != "" ? var.legacy_claims_table_name : "${terraform.workspace == "prod" ? "prod" : "staging"}-content-claims-claims-v1"
    inferred_legacy_claims_table_region = var.legacy_claims_table_region != "" ? var.legacy_claims_table_region : "${terraform.workspace == "prod" ? "us-west-2" : "us-east-2"}"
    inferred_legacy_claims_bucket_name = var.legacy_claims_bucket_name != "" ? var.legacy_claims_bucket_name : "${terraform.workspace == "prod" ? "prod-content-claims-bucket-claimsv1bucketefd46802-1mqz6d8o7xw8" : "staging-content-claims-buc-claimsv1bucketefd46802-1xx2brszve6t3"}"
    inferred_legacy_block_index_table_name = var.legacy_block_index_table_name != "" ? var.legacy_block_index_table_name : "${terraform.workspace == "prod" ? "prod" : "staging"}-ep-v1-blocks-cars-position"
}

data "aws_s3_bucket" "legacy_claims_bucket" {
  bucket = local.inferred_legacy_claims_bucket_name
}

provider "aws" {
  alias = "legacy_claims"
  region = local.inferred_legacy_claims_table_region
}

data "aws_dynamodb_table" "legacy_claims_table" {
  provider = aws.legacy_claims
  name = local.inferred_legacy_claims_table_name
}

provider "aws" {
  alias = "block_index"
  region = var.legacy_block_index_table_region
}

data "aws_dynamodb_table" "legacy_block_index_table" {
  provider = aws.block_index
  name = local.inferred_legacy_block_index_table_name
}

data "aws_iam_policy_document" "lambda_legacy_dynamodb_query_document" {
  statement {
    actions = [
      "dynamodb:Query",
    ]
    resources = [
      data.aws_dynamodb_table.legacy_claims_table.arn,
      data.aws_dynamodb_table.legacy_block_index_table.arn
    ]
  }
}

resource "aws_iam_policy" "lambda_legacy_dynamodb_query" {
  name        = "${terraform.workspace}-${var.app}-lambda-legacy-dynamodb-query"
  description = "This policy will be used by the lambda to query data from legacy DynamoDB tables"
  policy      = data.aws_iam_policy_document.lambda_legacy_dynamodb_query_document.json
}

resource "aws_iam_role_policy_attachment" "lambda_legacy_dynamodb_query" {
  role       = aws_iam_role.lambda_exec.name
  policy_arn = aws_iam_policy.lambda_legacy_dynamodb_query.arn
}

data "aws_iam_policy_document" "lambda_legacy_s3_get_document" {
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

resource "aws_iam_policy" "lambda_legacy_s3_get" {
  name        = "${terraform.workspace}-${var.app}-lambda_legacy_s3_get"
  description = "This policy will be used by the lambda to get objects from legacy S3 buckets"
  policy      = data.aws_iam_policy_document.lambda_legacy_s3_get_document.json
}

resource "aws_iam_role_policy_attachment" "lambda_legacy_s3_get" {
  role       = aws_iam_role.lambda_exec.name
  policy_arn = aws_iam_policy.lambda_legacy_s3_get.arn
}
