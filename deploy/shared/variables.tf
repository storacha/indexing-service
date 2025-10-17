variable "app" {
  description = "The name of the application"
  type        = string
}

variable "allowed_account_id" {
  description = "account id used for AWS"
  type = string
}

variable "domain_base" {
  type = string
  default = ""
}

variable "create_shared_dev_resources" {
  description = "create shared resources (vpc, caches, db, kms) for dev environments"
  type = bool
  default = false
}


variable "cloudflare_zone_id" {
  type = string
}
