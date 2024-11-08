variable "app" {
  description = "The name of the application"
  type        = string
  default = "indexer"
}

variable "region" {
  description = "aws region for all services"
  type = string
  default = "us-west-2"
}

variable "allowed_account_ids" {
  description = "account ids used for AWS"
  type = list(string)
  default = ["505595374361"]
}

variable "private_key" {
  description = "private_key for the peer for this deployment"
  type = string
}