resource "aws_ssm_parameter" "private_key" {
  name        = "/${var.app}/${terraform.workspace}/Secret/PRIVATE_KEY/value"
  description = "private key for the deployed environment"
  type        = "SecureString"
  value       = var.private_key
}

resource "aws_ssm_parameter" "honeycomb_api_key" {
  name        = "/${var.app}/${terraform.workspace}/honeycomb_api_key"
  description = "Honeycomb ingestion API key to send traces"
  type        = "SecureString"
  value       = var.honeycomb_api_key
}
