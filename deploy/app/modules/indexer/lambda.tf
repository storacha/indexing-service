locals {
  functions = {
    getroot = {
      name = "GETroot"
    }
    getclaim = {
      name = "GETclaim"
    }
    getclaims = {
      name = "GETclaims"
    }
    postclaims = {
      name = "POSTclaims"
    }
    notifier = {
      name = "notifier"
    }
    providercache = {
      name = "providercache"
      timeout = 300
    }
    remotesync = {
      name = "remotesync"
    }
  }

  # use dedicated VPC and caches for prod and staging, and shared resources otherwise
  dedicated_resources = terraform.workspace == "prod" || terraform.workspace == "staging"
  vpc_id = local.dedicated_resources ? module.vpc[0].id : data.terraform_remote_state.shared.outputs.dev_vpc.id
  private_subnet_ids = local.dedicated_resources ? module.vpc[0].subnet_ids.private : data.terraform_remote_state.shared.outputs.dev_vpc.subnet_ids.private
  providers_cache = local.dedicated_resources ? module.caches[0].providers : data.terraform_remote_state.shared.outputs.dev_caches.providers
  indexes_cache = local.dedicated_resources ? module.caches[0].indexes : data.terraform_remote_state.shared.outputs.dev_caches.indexes
  claims_cache = local.dedicated_resources ? module.caches[0].claims : data.terraform_remote_state.shared.outputs.dev_caches.claims
  cache_iam_user = local.dedicated_resources ? module.caches[0].iam_user : data.terraform_remote_state.shared.outputs.dev_caches.iam_user
  cache_security_group_id = local.dedicated_resources ? module.caches[0].security_group_id : data.terraform_remote_state.shared.outputs.dev_caches.security_group_id
}

// zip the binary along with the config file for the otel collector
data "archive_file" "function_archive" {
  for_each = local.functions

  type        = "zip"
  source_dir = "${path.root}/../../build/${each.key}"
  output_path = "${path.root}/../../build/${each.key}/${each.key}.zip"
}

data "aws_region" "legacy_claims" {
  provider = aws.legacy_claims
}

data "aws_region" "block_index" {
  provider = aws.block_index
}

# Define functions
resource "aws_lambda_function" "lambda" {
  depends_on = [ aws_cloudwatch_log_group.lambda_log_group ]
  for_each = local.functions

  function_name = "${terraform.workspace}-${var.app}-lambda-${each.value.name}"
  handler       = "bootstrap"
  runtime       = "provided.al2023"
  architectures = [ "arm64" ]
  role          = aws_iam_role.lambda_exec.arn
  timeout          = try(each.value.timeout, 30)
  memory_size      = try(each.value.memory_size, 128)
  reserved_concurrent_executions = try(each.value.concurrency, -1)
  source_code_hash = data.archive_file.function_archive[each.key].output_base64sha256
  filename      = data.archive_file.function_archive[each.key].output_path # Path to your Lambda zip files
  layers = [
      "arn:aws:lambda:${data.aws_region.current.name}:901920570463:layer:aws-otel-collector-arm64-ver-0-115-0:2"
    ]

  tracing_config {
    mode = "PassThrough"
  }

  environment {
    variables = {
      	PROVIDERS_REDIS_URL = local.providers_cache.address
        PROVIDERS_REDIS_CACHE = local.providers_cache.name
        PROVIDERS_CACHE_EXPIRATION_SECONDS = "${terraform.workspace == "prod" ? 30 * 24 * 60 * 60 : 24 * 60 * 60}"
        INDEXES_REDIS_URL = local.indexes_cache.address
        INDEXES_REDIS_CACHE = local.indexes_cache.name
        INDEXES_CACHE_EXPIRATION_SECONDS = "${terraform.workspace == "prod" ? 24 * 60 * 60 : 60 * 60}"
        CLAIMS_REDIS_URL = local.claims_cache.address
        CLAIMS_REDIS_CACHE = local.claims_cache.name
        CLAIMS_CACHE_EXPIRATION_SECONDS = "${terraform.workspace == "prod" ? 7 * 24 * 60 * 60 : 24 * 60 * 60}"
        REDIS_USER_ID = local.cache_iam_user.user_id
        IPNI_ENDPOINT = "https://cid.contact"
        PROVIDER_CACHING_QUEUE_URL = aws_sqs_queue.caching.id
        PROVIDER_CACHING_BUCKET_NAME = aws_s3_bucket.caching_bucket.bucket
        CHUNK_LINKS_TABLE_NAME = aws_dynamodb_table.chunk_links.id
        METADATA_TABLE_NAME = aws_dynamodb_table.metadata.id
        IPNI_STORE_BUCKET_NAME = aws_s3_bucket.ipni_store_bucket.bucket
        NOTIFIER_HEAD_BUCKET_NAME = aws_s3_bucket.notifier_head_bucket.bucket
        NOTIFIER_SNS_TOPIC_ARN = aws_sns_topic.published_advertisememt_head_change.id
        PRIVATE_KEY = aws_ssm_parameter.private_key.name
        DID = var.did
        PUBLIC_URL = "https://${aws_apigatewayv2_domain_name.custom_domain.domain_name}"
        IPNI_STORE_BUCKET_REGIONAL_DOMAIN = aws_s3_bucket.ipni_store_bucket.bucket_regional_domain_name
        CLAIM_STORE_BUCKET_NAME = aws_s3_bucket.claim_store_bucket.bucket
        LEGACY_CLAIMS_TABLE_NAME = data.aws_dynamodb_table.legacy_claims_table.id
        LEGACY_CLAIMS_TABLE_REGION = data.aws_region.legacy_claims.name
        LEGACY_CLAIMS_BUCKET_NAME = data.aws_s3_bucket.legacy_claims_bucket.id
        LEGACY_BLOCK_INDEX_TABLE_NAME = data.aws_dynamodb_table.legacy_block_index_table.id
        LEGACY_BLOCK_INDEX_TABLE_REGION = data.aws_region.block_index.name
        LEGACY_DATA_BUCKET_URL = var.legacy_data_bucket_url != "" ? var.legacy_data_bucket_url : "https://carpark-${terraform.workspace == "prod" ? "prod" : "staging"}-0.r2.w3s.link"
        GOLOG_LOG_LEVEL = terraform.workspace == "prod" ? "error" : "debug"
        OTEL_PROPAGATORS = "tracecontext"
        OTEL_SERVICE_NAME = "${terraform.workspace}-${var.app}"
        OPENTELEMETRY_COLLECTOR_CONFIG_URI = "/var/task/otel-collector-config.yaml"
        HONEYCOMB_OTLP_ENDPOINT = "api.honeycomb.io:443"
        HONEYCOMB_API_KEY = "${var.honeycomb_api_key}"
        PRINCIPAL_MAPPING = var.principal_mapping
    }
  }

  vpc_config {
    security_group_ids = [aws_security_group.lambda_security_group.id]
    subnet_ids = local.private_subnet_ids
  }
}

# Acccess for the gateway

resource "aws_lambda_permission" "api_gateway" {
  for_each = aws_lambda_function.lambda

  statement_id  = "AllowAPIGatewayInvoke"
  action        = "lambda:InvokeFunction"
  function_name = each.value.function_name
  principal     = "apigateway.amazonaws.com"
  source_arn    = "${aws_apigatewayv2_api.api.execution_arn}/*/*"
}

# Logging

resource "aws_cloudwatch_log_group" "lambda_log_group" {
  for_each = local.functions
  name              = "/aws/lambda/${terraform.workspace}-${var.app}-lambda-${each.value.name}"
  retention_in_days = 7
  lifecycle {
    prevent_destroy = false
  }
}

# Role policies and access to resources

resource "aws_iam_role" "lambda_exec" {
  name = "${terraform.workspace}-lambda-exec-role"

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

resource "aws_iam_role_policy_attachment" "lambda_vpc_access_attachment" {
  role       = aws_iam_role.lambda_exec.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaVPCAccessExecutionRole"
}

data "aws_iam_policy_document" "lambda_elasticache_connect_document" {
  statement {
    effect = "Allow"
    actions = [
      "elasticache:Connect"
    ]

    resources = [
      local.providers_cache.arn,
      local.indexes_cache.arn,
      local.claims_cache.arn,
      local.cache_iam_user.arn,
    ]
  }
}

resource "aws_iam_policy" "lambda_elasticache_connect" {
  name   = "${terraform.workspace}-${var.app}-lambda-elasticache-connect"
  policy = data.aws_iam_policy_document.lambda_elasticache_connect_document.json
}

resource "aws_iam_role_policy_attachment" "lambda_elasticache_connect" {
  policy_arn = aws_iam_policy.lambda_elasticache_connect.arn
  role       = aws_iam_role.lambda_exec.name
}

data "aws_iam_policy_document" "lambda_dynamodb_put_get_document" {
  statement {
    actions = [
      "dynamodb:GetItem",
      "dynamodb:PutItem",
    ]
    resources = [
      aws_dynamodb_table.chunk_links.arn,
      aws_dynamodb_table.metadata.arn
    ]
  }
}

resource "aws_iam_policy" "lambda_dynamodb_put_get" {
  name        = "${terraform.workspace}-${var.app}-lambda-dynamodb-put-get"
  description = "This policy will be used by the lambda to put and get data from DynamoDB"
  policy      = data.aws_iam_policy_document.lambda_dynamodb_put_get_document.json
}

resource "aws_iam_role_policy_attachment" "lambda_dynamodb_put_get" {
  role       = aws_iam_role.lambda_exec.name
  policy_arn = aws_iam_policy.lambda_dynamodb_put_get.arn
}


data "aws_iam_policy_document" "lambda_s3_put_get_document" {
  statement {
    actions = [
      "s3:GetObject",
      "s3:PutObject",
      "s3:DeleteObject",
    ]
    resources = [
      "${aws_s3_bucket.caching_bucket.arn}/*",
      "${aws_s3_bucket.ipni_store_bucket.arn}/*",
      "${aws_s3_bucket.notifier_head_bucket.arn}/*",
      "${aws_s3_bucket.claim_store_bucket.arn}/*"
    ]
  }
  statement {
    actions = [
      "s3:ListBucket","s3:GetBucketLocation"
    ]
    resources = [
      aws_s3_bucket.caching_bucket.arn,
      aws_s3_bucket.ipni_store_bucket.arn,
      aws_s3_bucket.notifier_head_bucket.arn,
      aws_s3_bucket.claim_store_bucket.arn
    ]
  }
}

resource "aws_iam_policy" "lambda_s3_put_get" {
  name        = "${terraform.workspace}-${var.app}-lambda-s3-put-get"
  description = "This policy will be used by the lambda to put and get objects from S3"
  policy      = data.aws_iam_policy_document.lambda_s3_put_get_document.json
}

resource "aws_iam_role_policy_attachment" "lambda_s3_put_get" {
  role       = aws_iam_role.lambda_exec.name
  policy_arn = aws_iam_policy.lambda_s3_put_get.arn
}

data "aws_iam_policy_document" "lambda_logs_document" {
  statement {
    actions = [
      "logs:CreateLogStream",
      "logs:PutLogEvents",
    ]
    resources = [for k, group in aws_cloudwatch_log_group.lambda_log_group : group.arn ]
  }
}

resource "aws_iam_policy" "lambda_logs" {
  name        = "${terraform.workspace}-${var.app}-lambda-logs"
  description = "This policy will be used by the lambda to write logs"
  policy      = data.aws_iam_policy_document.lambda_logs_document.json
}

resource "aws_iam_role_policy_attachment" "lambda_logs" {
  role       = aws_iam_role.lambda_exec.name
  policy_arn = aws_iam_policy.lambda_logs.arn
}

data "aws_iam_policy_document" "lambda_sns_document" {
  statement {
    actions = [
      "sns:Publish"
    ]
    resources = [
      aws_sns_topic.published_advertisememt_head_change.arn
    ]
  }
}

resource "aws_iam_policy" "lambda_sns" {
  name        = "${terraform.workspace}-${var.app}-lambda-sns"
  description = "This policy will be used by the lambda to push to sns"
  policy      = data.aws_iam_policy_document.lambda_sns_document.json
}

resource "aws_iam_role_policy_attachment" "lambda_sns" {
  role       = aws_iam_role.lambda_exec.name
  policy_arn = aws_iam_policy.lambda_sns.arn
}

data "aws_iam_policy_document" "lambda_ssm_document" {
  statement {
  
    effect = "Allow"
  
    actions = [
      "ssm:GetParameter",
    ]

    resources = [
      aws_ssm_parameter.private_key.arn
    ]
  }
}

resource "aws_iam_policy" "lambda_ssm" {
  name        = "${terraform.workspace}-${var.app}-lambda-ssm"
  description = "This policy will be used by the lambda to access the parameter store"
  policy      = data.aws_iam_policy_document.lambda_ssm_document.json
}

resource "aws_iam_role_policy_attachment" "lambda_ssm" {
  role       = aws_iam_role.lambda_exec.name
  policy_arn = aws_iam_policy.lambda_ssm.arn
}

data "aws_iam_policy_document" "lambda_sqs_document" {
  statement {
  
    effect = "Allow"
  
    actions = [
      "sqs:SendMessage*",
      "sqs:ReceiveMessage",
      "sqs:DeleteMessage",
      "sqs:GetQueueAttributes"
    ]

    resources = [
      aws_sqs_queue.caching.arn
    ]
  }
}

resource "aws_iam_policy" "lambda_sqs" {
  name        = "${terraform.workspace}-${var.app}-lambda-sqs"
  description = "This policy will be used by the lambda to send messages to an SQS queue"
  policy      = data.aws_iam_policy_document.lambda_sqs_document.json
}

resource "aws_iam_role_policy_attachment" "lambda_sqs" {
  role       = aws_iam_role.lambda_exec.name
  policy_arn = aws_iam_policy.lambda_sqs.arn
}

# event source mappings

resource "aws_lambda_event_source_mapping" "event_source_mapping" {
  event_source_arn = aws_sqs_queue.caching.arn
  enabled          = true
  function_name    = aws_lambda_function.lambda["providercache"].arn
  batch_size       = terraform.workspace == "prod" ? 10 : 1
}

resource "aws_cloudwatch_event_rule" "head_check" {
  name                = "${terraform.workspace}-${var.app}-lambda-head-check"
  description         = "Fires every minute"
  schedule_expression = "cron(* * * * ? *)"
}

resource "aws_cloudwatch_event_target" "notifier" {
  rule      = aws_cloudwatch_event_rule.head_check.name
  target_id = "${terraform.workspace}-${var.app}-lambda-notifier-target"
  arn       = aws_lambda_function.lambda["notifier"].arn
}

resource "aws_lambda_permission" "allow_cloudwatch_to_call_notifier" {
  statement_id  = "AllowExecutionFromCloudWatch"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.lambda["notifier"].function_name
  principal     = "events.amazonaws.com"
  source_arn    = aws_cloudwatch_event_rule.head_check.arn
}

resource "aws_sns_topic_subscription" "invoke_with_sns" {
  topic_arn = aws_sns_topic.published_advertisememt_head_change.arn
  protocol  = "lambda"
  endpoint  = aws_lambda_function.lambda["remotesync"].arn
}

resource "aws_lambda_permission" "allow_sns_invoke" {
  statement_id  = "AllowExecutionFromSNS"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.lambda["remotesync"].function_name
  principal     = "sns.amazonaws.com"
  source_arn    = aws_sns_topic.published_advertisememt_head_change.arn
}

# VPC Access

resource "aws_security_group" "lambda_security_group" {
  name        = "${terraform.workspace}-${var.app}-lambda-security-group"
  description = ("Allow traffic from lambda to elasticache")
  vpc_id      = local.vpc_id

  egress {
    from_port       = 6379
    to_port         = 6380
    protocol        = "tcp"
    description     = "Allow elasticache access"
    security_groups = [local.cache_security_group_id]
  }

  egress {
    from_port = 443
    to_port   = 443
    protocol  = "tcp"
    description = "Allow internet access"
    cidr_blocks = ["0.0.0.0/0"]
  }
}