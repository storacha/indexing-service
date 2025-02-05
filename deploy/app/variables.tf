variable "app" {
  description = "The name of the application"
  type        = string
  default = "indexer"
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

variable "did" {
  description = "DID for this deployment (did:web:... for example)"
  type = string
}

variable "honeycomb_api_key" {
  description = "Ingestion API key to send traces to Honeycomb"
  type = string
  default = ""
}
