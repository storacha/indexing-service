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


variable "peerID" {
  description = "peerID for ipni"
  type = string
}