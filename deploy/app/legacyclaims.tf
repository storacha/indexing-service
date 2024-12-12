variable "legacy_claims_table_name" {
  description = "The name of the DynamoDB table used by the legacy content-claims service"
  type = string
  default = "prod-content-claims-claims-v1"
}

variable "legacy_claims_bucket_name" {
  description = "The name of the S3 bucket used by the legacy content-claims service"
  type = string
  default = "prod-content-claims-bucket-claimsv1bucketefd46802-1mqz6d8o7xw8"
}

variable "legacy_block_index_table_name" {
  description = "The name of the legacy block index DynamoDB table"
  type = string
  default = ""
}

locals {
    inferred_legacy_block_index_table_name = var.legacy_block_index_table_name != "" ? var.legacy_block_index_table_name : "${terraform.workspace == "prod" ? "prod" : "staging"}-ep-v1-blocks-cars-position"
}

data "aws_s3_bucket" "legacy_claims_bucket" {
  bucket = var.legacy_claims_bucket_name
}

data "aws_dynamodb_table" "legacy_claims_table" {
  name = var.legacy_claims_table_name
}

data "aws_dynamodb_table" "legacy_block_index_table" {
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
