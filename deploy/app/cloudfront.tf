locals {
  # Only prod gets a CloudFront distribution
  should_create_cloudfront = terraform.workspace == "prod"
}

resource "aws_cloudfront_distribution" "indexer" {
  count = local.should_create_cloudfront ? 1 : 0

  origin {
    domain_name = "${var.app}.storacha.network"
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

    # Managed policy AllViewerExceptHostHeader: forward all parameters in viewer requests except for the Host header
    origin_request_policy_id = "b689b0a8-53d0-40ab-baf2-68738e2966ac"

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
    acm_certificate_arn            = aws_acm_certificate.cert.arn
    ssl_support_method             = "sni-only"
    minimum_protocol_version       = "TLSv1.2_2021"
  }

  aliases = ["accelerated.${var.app}.storacha.network"]
}


resource "aws_acm_certificate" "cert" {
  domain_name       = "accelerated.${var.app}.storacha.network"
  validation_method = "DNS"
  
  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_route53_record" "cert_validation" {
  allow_overwrite = true
  name    = tolist(aws_acm_certificate.cert.domain_validation_options)[0].resource_record_name
  type    = tolist(aws_acm_certificate.cert.domain_validation_options)[0].resource_record_type
  zone_id = aws_route53_zone.primary.zone_id
  records = [tolist(aws_acm_certificate.cert.domain_validation_options)[0].resource_record_value]
  ttl     = 60
}

resource "aws_acm_certificate_validation" "cert" {
  certificate_arn = aws_acm_certificate.cert.arn
  validation_record_fqdns = [ aws_route53_record.cert_validation.fqdn ]
}
