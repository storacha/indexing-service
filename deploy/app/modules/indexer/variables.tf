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

variable "private_key" {
  description = "private_key for the peer for this deployment"
  type        = string
}

variable "did" {
  description = "DID for this deployment (did:web:... for example)"
  type        = string
}

variable "honeycomb_api_key" {
  description = "Ingestion API key to send traces to Honeycomb"
  type        = string
  default     = ""
}

variable "principal_mapping" {
  type        = string
  description = "JSON encoded mapping of did:web to did:key"
  default     = ""
}

variable "legacy_data_bucket_url" {
  type        = string
  description = "URL to use when constructing synthesizing legacy claims"
  default     = ""
}
