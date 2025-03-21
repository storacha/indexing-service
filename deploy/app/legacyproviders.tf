variable "legacy_claims_table_region" {
  description = "The region where the legacy content-claims DynamoDB table is provisioned"
  type = string
  default = ""
}

# the block index table is always deployed in us-west-2 for both prod and staging
variable "legacy_block_index_table_region" {
  description = "The region where the legacy block index DynamoDB table is provisioned"
  type = string
  default = "us-west-2"
}

locals {
    inferred_legacy_claims_table_region = var.legacy_claims_table_region != "" ? var.legacy_claims_table_region : "${terraform.workspace == "prod" ? "us-west-2" : "us-east-2"}"
}

provider "aws" {
  alias = "legacy_claims"
  region = local.inferred_legacy_claims_table_region
}

provider "aws" {
  alias = "block_index"
  region = var.legacy_block_index_table_region
}