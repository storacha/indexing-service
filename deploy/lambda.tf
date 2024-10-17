locals {
  functions = {
    getroot = {
      name = "GETroot"
    }
  }
}

// zip the binary, as we can use only zip files to AWS lambda
data "archive_file" "function_archive" {
  for_each = local.functions

  type        = "zip"
  source_file = "build/${each.key}/boostrap"
  output_path = "build/${each.key}/${each.key}.zip"
}


resource "aws_lambda_function" "lambda" {
  for_each = local.functions

  function_name = "${terraform.workspace}-${var.app}-lambda-${each.value.name}"
  handler       = "bootstrap"
  runtime       = "provided.al2023"
  role          = aws_iam_role.lambda_exec.arn
  timeout          = try(each.value.timeout, 3)
  memory_size      = try(each.value.memory_size, 128)
  reserved_concurrent_executions = try(each.value.concurrency, -1)
  filename      = data.archive_file.function_archive[each.key].output_path # Path to your Lambda zip files

  environment {
    variables = {
      	PROVIDERS_REDIS_URL = aws_elasticache_serverless_cache.cache["providers"].endpoint
        PROVIDERS_REDIS_CACHE = aws_elasticache_serverless_cache.cache["providers"].name
        INDEXES_REDIS_URL = aws_elasticache_serverless_cache.cache["indexes"].endpoint
        INDEXES_REDIS_CACHE = aws_elasticache_serverless_cache.cache["indexes"].name
        CLAIMS_REDIS_URL = aws_elasticache_serverless_cache.cache["claims"].endpoint
        CLAIMS_REDIS_CACHE = aws_elasticache_serverless_cache.cache["claims"].name
        REDIS_USER_ID = aws_elasticache_user.cache_iam_user.user_id
        IPNI_ENDPOINT = "https://cid.contact"
        IPNI_PUBLISHER_ANNOUNCE_ADDRESS = "/dns4/${aws_s3_bucket.ipni_store_bucket.bucket_regional_domain_name}/tcp/443/https/p2p/${var.peerID}"
        PROVIDER_CACHING_QUEUE_URL = aws_sqs_queue.caching.id
        PROVIDER_CACHING_BUCKET_NAME = aws_s3_bucket.caching_bucket.bucket
        CHUNK_LINKS_TABLE_NAME = aws_dynamodb_table.chunk_links.id
        METADATA_TABLE_NAME = aws_dynamodb_table.metadata.id
        IPNI_STORE_BUCKET_NAME = aws_s3_bucket.ipni_store_bucket.bucket
        NOTIFIER_HEAD_BUCKET_NAME = aws_s3_bucket.notifier_head_bucket.bucket
        NOTIFIER_HEAD_BUCKET_NAME = aws_sns_topic.published_advertisememt_head_change.id
    }
  }
}

# resource "aws_lambda_permission" "api_gateway" {
#   for_each = aws_lambda_function.lambda

#   statement_id  = "AllowAPIGatewayInvoke"
#   action        = "lambda:InvokeFunction"
#   function_name = each.value.function_name
#   principal     = "apigateway.amazonaws.com"
#   source_arn    = "${aws_api_gateway_rest_api.api.execution_arn}/*/*"
#}

resource "aws_iam_role" "lambda_exec" {
  name = "lambda_exec_role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          Service = "lambda.amazonaws.com"
        }
      }
    ]
  })
}

resource "aws_iam_role_policy_attachment" "lambda_exec_policy" {
  role       = aws_iam_role.lambda_exec.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
}