variable "app" {
  description = "The name of the application"
  type        = string
}

variable "environment" {
  description = "The environment the caches will belong to"
  type        = string
}

variable "cache_limits" {
  description = "The caches usage limits"
  type        = object({
    data_storage_GB = number
    ecpu_per_second = number
  })
}

variable "vpc" {
  description = "The VPC to deploy the caches in"
  type        = object({
    id                 = string
    cidr_block         = string
    private_subnet_ids = list(string)
  })
}
