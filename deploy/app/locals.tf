locals {
  is_production = terraform.workspace == "prod" || startswith(terraform.workspace, "prod-") || endswith(terraform.workspace, "-prod")

  deployment_config = local.is_production ? {
    cpu = 2048
    memory = 4096
    service_min = 4
    service_max = 10
    httpport = 8080
    readonly = true
  } : {
    cpu = 256
    memory = 512
    service_min = 1
    service_max = 2
    httpport = 8080
    readonly = true
  }
}
