resource "aws_api_gateway_rest_api" "api" {
  name        = "${terraform.workspace}-${var.app}-api"
  description = "${terraform.workspace} ${var.app} API Gateway"
}

resource "aws_api_gateway_resource" "claims" {
  rest_api_id = aws_api_gateway_rest_api.api.id
  parent_id   = aws_api_gateway_rest_api.api.root_resource_id
  path_part   = "claims"
}

resource "aws_api_gateway_method" "getroot" {
  rest_api_id   = aws_api_gateway_rest_api.api.id
  resource_id   = aws_api_gateway_rest_api.api.root_resource_id
  http_method   = "GET"
  authorization = "NONE"
}

resource "aws_api_gateway_method" "getclaims" {
  rest_api_id   = aws_api_gateway_rest_api.api.id
  resource_id   = aws_api_gateway_resource.claims.id
  http_method   = "GET"
  authorization = "NONE"
}

resource "aws_api_gateway_method" "postclaims" {
  rest_api_id   = aws_api_gateway_rest_api.api.id
  resource_id   = aws_api_gateway_resource.claims.id
  http_method   = "POST"
  authorization = "NONE"
}

resource "aws_api_gateway_integration" "getroot" {
  rest_api_id             = aws_api_gateway_rest_api.api.id
  resource_id             = aws_api_gateway_method.getroot.resource_id
  http_method             = aws_api_gateway_method.getroot.http_method
  integration_http_method = "POST"
  type                    = "AWS_PROXY"
  uri                     = "tbd"
}

resource "aws_api_gateway_integration" "getclaims" {
  rest_api_id             = aws_api_gateway_rest_api.api.id
  resource_id             = aws_api_gateway_method.getclaims.resource_id
  http_method             = aws_api_gateway_method.getclaims.http_method
  integration_http_method = "POST"
  type                    = "AWS_PROXY"
  uri                     = "tbd" #aws_lambda_function.lambda.invoke_arn
}

resource "aws_api_gateway_integration" "postclaims" {
  rest_api_id             = aws_api_gateway_rest_api.api.id
  resource_id             = aws_api_gateway_method.postclaims.resource_id
  http_method             = aws_api_gateway_method.postclaims.http_method
  integration_http_method = "POST"
  type                    = "AWS_PROXY"
  uri                     = "tbd"
}

resource "aws_api_gateway_deployment" "deployment" {
  depends_on = [aws_api_gateway_integration.getroot, aws_api_gateway_integration.getclaims,aws_api_gateway_method.postclaims]

  rest_api_id = aws_api_gateway_rest_api.api.id
  stage_name  = "prod"
}

resource "aws_acm_certificate" "cert" {
  domain_name       = "indexer.storacha.network"
  validation_method = "DNS"

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_route53_zone" "primary" {
  name = "storacha.network"
}

resource "aws_route53_record" "cert_validation" {
  name    = aws_acm_certificate.cert.domain_validation_options[0].resource_record_name
  type    = aws_acm_certificate.cert.domain_validation_options[0].resource_record_type
  zone_id = aws_route53_zone.primary.zone_id
  records = [aws_acm_certificate.cert.domain_validation_options[0].resource_record_value]
  ttl     = 60
}

resource "aws_api_gateway_domain_name" "custom_domain" {
  domain_name = "${terraform.workspace}.${var.app}.storacha.network"
  certificate_arn = aws_acm_certificate.cert.arn
}

resource "aws_api_gateway_base_path_mapping" "path_mapping" {
  api_id      = aws_api_gateway_rest_api.api.id
  stage_name  = aws_api_gateway_deployment.deployment.stage_name
  domain_name = aws_api_gateway_domain_name.custom_domain.domain_name
}

resource "aws_route53_record" "api_gateway" {
  zone_id = aws_route53_zone.primary.zone_id
  name    = "${terraform.workspace}.${var.app}.storacha.network"
  type    = "A"

  alias {
    name                   = aws_api_gateway_domain_name.custom_domain.cloudfront_domain_name
    zone_id                = aws_api_gateway_domain_name.custom_domain.cloudfront_zone_id
    evaluate_target_health = false
  }
}