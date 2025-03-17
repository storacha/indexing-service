variable "app" {
  description = "The name of the application"
  type        = string
  default     = "indexer"
}

variable "allowed_account_ids" {
  description = "account ids used for AWS"
  type        = list(string)
  default     = ["505595374361"]
}
