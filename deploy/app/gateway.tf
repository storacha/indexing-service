locals {
    domain_name = terraform.workspace == "prod" ? "api.${var.app}.storacha.network" : (terraform.workspace == "staging" ? "api.staging.${var.app}.storacha.network" : "${terraform.workspace}.${var.app}.storacha.network")
}

resource "aws_apigatewayv2_api" "api" {
  name        = "${terraform.workspace}-${var.app}-api"
  description = "${terraform.workspace} ${var.app} API Gateway"
  protocol_type = "HTTP"
}

resource "aws_apigatewayv2_route" "getclaim" {
  api_id =  aws_apigatewayv2_api.api.id
  route_key = "GET /claim/{cid}"
  authorization_type = "NONE"
  target = "integrations/${aws_apigatewayv2_integration.getclaim.id}"
}

resource "aws_apigatewayv2_route" "getclaims" {
  api_id =  aws_apigatewayv2_api.api.id
  route_key = "GET /claims"
  authorization_type = "NONE"
  target = "integrations/${aws_apigatewayv2_integration.getclaims.id}"
}

resource "aws_apigatewayv2_route" "getroot" {
  api_id =  aws_apigatewayv2_api.api.id
  route_key = "GET /"
  authorization_type = "NONE"
  target = "integrations/${aws_apigatewayv2_integration.getroot.id}"
}

resource "aws_apigatewayv2_route" "postclaims" {
  api_id =  aws_apigatewayv2_api.api.id
  route_key = "POST /claims"
  authorization_type = "NONE"
  target = "integrations/${aws_apigatewayv2_integration.postclaims.id}"
}

resource "aws_apigatewayv2_integration" "getclaim" {
  api_id             = aws_apigatewayv2_api.api.id
  integration_uri =  aws_lambda_function.lambda["getclaim"].invoke_arn
  payload_format_version = "2.0"
  integration_type    = "AWS_PROXY"
  connection_type = "INTERNET"
}

resource "aws_apigatewayv2_integration" "getclaims" {
  api_id             = aws_apigatewayv2_api.api.id
  integration_uri =  aws_lambda_function.lambda["getclaims"].invoke_arn
  payload_format_version = "2.0"
  integration_type    = "AWS_PROXY"
  connection_type = "INTERNET"
}


resource "aws_apigatewayv2_integration" "getroot" {
  api_id             = aws_apigatewayv2_api.api.id
  integration_uri =  aws_lambda_function.lambda["getroot"].invoke_arn
  payload_format_version = "2.0"
  integration_type    = "AWS_PROXY"
  connection_type = "INTERNET"
}


resource "aws_apigatewayv2_integration" "postclaims" {
  api_id             = aws_apigatewayv2_api.api.id
  integration_uri =  aws_lambda_function.lambda["postclaims"].invoke_arn
  payload_format_version = "2.0"
  integration_type    = "AWS_PROXY"
  connection_type = "INTERNET"
}

resource "aws_apigatewayv2_deployment" "deployment" {
  depends_on = [aws_apigatewayv2_integration.getclaim, aws_apigatewayv2_integration.getclaims, aws_apigatewayv2_integration.getroot, aws_apigatewayv2_integration.postclaims]
  triggers = {
    redeployment = sha1(join(",", [
      jsonencode(aws_apigatewayv2_integration.getclaim),
      jsonencode(aws_apigatewayv2_integration.getclaims),
      jsonencode(aws_apigatewayv2_integration.postclaims),
      jsonencode(aws_apigatewayv2_integration.getroot),
      jsonencode(aws_apigatewayv2_route.getclaim),
      jsonencode(aws_apigatewayv2_route.getclaims),
      jsonencode(aws_apigatewayv2_route.getroot),
      jsonencode(aws_apigatewayv2_route.postclaims),
    ]))
  }

  api_id = aws_apigatewayv2_api.api.id
  description = "${terraform.workspace} ${var.app} API Deployment"
}

resource "aws_acm_certificate" "cert" {
  domain_name       = local.domain_name
  validation_method = "DNS"
  
  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_route53_record" "cert_validation" {
  allow_overwrite = true
  name    = tolist(aws_acm_certificate.cert.domain_validation_options)[0].resource_record_name
  type    = tolist(aws_acm_certificate.cert.domain_validation_options)[0].resource_record_type
  zone_id = data.terraform_remote_state.shared.outputs.primary_zone.zone_id
  records = [tolist(aws_acm_certificate.cert.domain_validation_options)[0].resource_record_value]
  ttl     = 60
}

resource "aws_acm_certificate_validation" "cert" {
  certificate_arn = aws_acm_certificate.cert.arn
  validation_record_fqdns = [ aws_route53_record.cert_validation.fqdn ]
}

resource "aws_apigatewayv2_domain_name" "custom_domain" {
  domain_name = local.domain_name

  domain_name_configuration {
    certificate_arn = aws_acm_certificate_validation.cert.certificate_arn
    endpoint_type = "REGIONAL"
    security_policy = "TLS_1_2"
  }
}

resource "aws_apigatewayv2_stage" "stage" {
  api_id = aws_apigatewayv2_api.api.id
  name   = "$default"
  deployment_id = aws_apigatewayv2_deployment.deployment.id
}

resource "aws_apigatewayv2_api_mapping" "api_mapping" {
  api_id      = aws_apigatewayv2_api.api.id
  stage       = aws_apigatewayv2_stage.stage.id
  domain_name = aws_apigatewayv2_domain_name.custom_domain.id
}

resource "aws_route53_record" "api_gateway" {
  zone_id = data.terraform_remote_state.shared.outputs.primary_zone.zone_id
  name    = aws_apigatewayv2_domain_name.custom_domain.domain_name
  type    = "A"

  alias {
    name                   = aws_apigatewayv2_domain_name.custom_domain.domain_name_configuration[0].target_domain_name
    zone_id                = aws_apigatewayv2_domain_name.custom_domain.domain_name_configuration[0].hosted_zone_id
    evaluate_target_health = false
  }
}
