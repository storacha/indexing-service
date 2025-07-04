resource "aws_dynamodb_table" "metadata" {
  name         = "${terraform.workspace}-${var.app}-metadata"
  billing_mode = "PAY_PER_REQUEST"

  attribute {
    name = "provider"
    type = "S"
  }

  attribute {
    name = "contextID"
    type = "B"
  }

  hash_key  = "provider"
  range_key = "contextID"

  tags = {
    Name = "${terraform.workspace}-${var.app}-metadata"
  }

  point_in_time_recovery {
    enabled = local.is_production
  }

  deletion_protection_enabled = local.is_production
}

resource "aws_dynamodb_table" "chunk_links" {
  name         = "${terraform.workspace}-${var.app}-chunk-links"
  billing_mode = "PAY_PER_REQUEST"

  attribute {
    name = "provider"
    type = "S"
  }

  attribute {
    name = "contextID"
    type = "B"
  }

  hash_key  = "provider"
  range_key = "contextID"

  tags = {
    Name = "${terraform.workspace}-${var.app}-chunk-links"
  }

  point_in_time_recovery {
    enabled = local.is_production
  }

  deletion_protection_enabled = local.is_production
}