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
    }
    remotesync = {
      name = "remotesync"
    }
  }
}

// zip the binary, as we can use only zip files to AWS lambda
data "archive_file" "function_archive" {
  for_each = local.functions

  type        = "zip"
  source_file = "${path.root}/../../build/${each.key}/bootstrap"
  output_path = "${path.root}/../../build/${each.key}/${each.key}.zip"
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

  environment {
    variables = {
      	PROVIDERS_REDIS_URL = aws_elasticache_serverless_cache.cache["providers"].endpoint[0].address
        PROVIDERS_REDIS_CACHE = aws_elasticache_serverless_cache.cache["providers"].name
        INDEXES_REDIS_URL = aws_elasticache_serverless_cache.cache["indexes"].endpoint[0].address
        INDEXES_REDIS_CACHE = aws_elasticache_serverless_cache.cache["indexes"].name
        CLAIMS_REDIS_URL = aws_elasticache_serverless_cache.cache["claims"].endpoint[0].address
        CLAIMS_REDIS_CACHE = aws_elasticache_serverless_cache.cache["claims"].name
        REDIS_USER_ID = aws_elasticache_user.cache_iam_user.user_id
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
    }
  }

  vpc_config {
    security_group_ids = [
      aws_security_group.lambda_security_group.id
    ]
    subnet_ids = aws_subnet.vpc_private_subnet[*].id
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
      "arn:aws:elasticache:${var.region}:${var.allowed_account_ids[0]}:serverlesscache:${aws_elasticache_serverless_cache.cache["providers"].id}",
      "arn:aws:elasticache:${var.region}:${var.allowed_account_ids[0]}:serverlesscache:${aws_elasticache_serverless_cache.cache["indexes"].id}",
      "arn:aws:elasticache:${var.region}:${var.allowed_account_ids[0]}:serverlesscache:${aws_elasticache_serverless_cache.cache["claims"].id}",
      "arn:aws:elasticache:${var.region}:${var.allowed_account_ids[0]}:user:${aws_elasticache_user.cache_iam_user.user_id}"
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
  vpc_id      = aws_vpc.vpc.id

  egress {
    from_port = 6379
    to_port   = 6379
    protocol  = "tcp"
    description = "Allow elasticache access"
    security_groups = [
      aws_security_group.cache_security_group.id,
    ]
  }

  egress {
    from_port = 443
    to_port   = 443
    protocol  = "tcp"
    description = "Allow internet access"
    cidr_blocks = ["0.0.0.0/0"]
  }
}