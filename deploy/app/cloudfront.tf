locals {
  # Only prod and staging get a CloudFront distribution
  should_create_cloudfront = local.is_production || local.is_staging
}

resource "aws_cloudfront_distribution" "indexer" {
  count = local.should_create_cloudfront ? 1 : 0

  origin {
    domain_name = aws_apigatewayv2_domain_name.custom_domain.domain_name_configuration[0].target_domain_name
    origin_id   = "indexer-origin"

    custom_origin_config {
      http_port              = 80
      https_port             = 443
      origin_protocol_policy = "https-only"
      origin_ssl_protocols = ["TLSv1.2"]
    }
  }

  enabled             = true
  is_ipv6_enabled     = true
  default_root_object = ""

  default_cache_behavior {
    target_origin_id       = "indexer-origin"
    viewer_protocol_policy = "redirect-to-https"

    allowed_methods = ["GET", "HEAD", "OPTIONS", "PUT", "POST", "PATCH", "DELETE"]
    cached_methods = ["GET", "HEAD"]

    # Managed policy AllViewer: forward all parameters in viewer requests
    origin_request_policy_id = "216adef6-5c7f-47e4-b989-5492eafa07d3"

    # Managed policy CachingDisabled: policy with caching disabled
    cache_policy_id  = "4135ea2d-6df8-44a3-9df3-4b5a84be39ad"
  }

  price_class = "PriceClass_All"

  restrictions {
    geo_restriction {
      restriction_type = "none"
    }
  }

  viewer_certificate {
    acm_certificate_arn            = aws_acm_certificate.cloudfront_cert[0].arn
    ssl_support_method             = "sni-only"
    minimum_protocol_version       = "TLSv1.2_2021"
  }

  aliases = [aws_apigatewayv2_domain_name.custom_domain.domain_name]
}

# CloudFront is a global service. Certs must be created in us-east-1, where the core ACM infra lives
provider "aws" {
  region = "us-east-1"
  alias = "acm"
}

resource "aws_acm_certificate" "cloudfront_cert" {
  count = local.should_create_cloudfront ? 1 : 0

  provider = aws.acm

  domain_name       = aws_apigatewayv2_domain_name.custom_domain.domain_name
  validation_method = "DNS"
  
  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_route53_record" "cloudfront_cert_validation" {
  count = local.should_create_cloudfront ? 1 : 0

  allow_overwrite = true
  name    = tolist(aws_acm_certificate.cloudfront_cert[0].domain_validation_options)[0].resource_record_name
  type    = tolist(aws_acm_certificate.cloudfront_cert[0].domain_validation_options)[0].resource_record_type
  zone_id = local.dns_zone_id
  records = [tolist(aws_acm_certificate.cloudfront_cert[0].domain_validation_options)[0].resource_record_value]
  ttl     = 60
}

resource "aws_acm_certificate_validation" "cloudfront_cert" {
  count = local.should_create_cloudfront ? 1 : 0

  provider = aws.acm

  certificate_arn = aws_acm_certificate.cloudfront_cert[0].arn
  validation_record_fqdns = [ aws_route53_record.cloudfront_cert_validation[0].fqdn ]
}
