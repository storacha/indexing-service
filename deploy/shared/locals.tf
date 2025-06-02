locals {
  is_production = terraform.workspace == "prod" || terraform.workspace == "warm-prod"
  is_staging = terraform.workspace == "staging" || terraform.workspace == "warm-staging"

  is_warm = startswith(terraform.workspace, "warm-")
  network = local.is_warm ? "warm.storacha.network" : "storacha.network"
}
