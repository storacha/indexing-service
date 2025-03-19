variable "app" {
  description = "The name of the application"
  type        = string
}

variable "environment" {
  description = "The environment the caches will belong to"
  type        = string
}

variable "node_type" {
  description = "The type of nodes to use for the cache"
  type        = string
}

variable "vpc" {
  description = "The VPC to deploy the caches in"
  type        = object({
    id                 = string
    cidr_block         = string
    private_subnet_ids = list(string)
  })
}
