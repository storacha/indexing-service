resource "aws_s3_bucket" "caching_bucket" {
  bucket = "${terraform.workspace}-${var.app}-caching-bucket"
}

resource "aws_s3_bucket" "ipni_store_bucket" {
  bucket = "${terraform.workspace}-${var.app}-ipni-store-bucket"
}

resource "aws_s3_bucket_cors_configuration" "ipni_store_cors" {
  bucket = aws_s3_bucket.ipni_store_bucket.bucket

  cors_rule {
    allowed_headers = ["*"]
    allowed_methods = ["GET", "HEAD"]
    allowed_origins = ["*"]
    expose_headers  = ["Content-Length", "Content-Type", "Content-MD5", "ETag"]
    max_age_seconds = 86400
  }
}

resource "aws_s3_bucket_policy" "ipni_store_policy" {
  bucket = aws_s3_bucket.ipni_store_bucket.id

  policy = jsonencode({
    "Version" : "2012-10-17",
    "Statement" : [
      {
        "Sid" : "PublicRead",
        "Effect" : "Allow",
        "Principal" : "*",
        "Action" : ["s3:GetObject", "s3:GetObjectVersion"],
        "Resource" : ["${aws_s3_bucket.ipni_store_bucket.arn}/*"]
      }
    ]
  })
}

resource "aws_s3_bucket" "notifier_head_bucket" {
  bucket = "${terraform.workspace}-${var.app}-notifier-head-bucket"
}