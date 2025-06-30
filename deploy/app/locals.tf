locals {
    is_production = terraform.workspace == "prod" || terraform.workspace == "warm-prod"
    is_staging = terraform.workspace == "staging" || terraform.workspace == "warm-staging"
    is_warm = startswith(terraform.workspace, "warm-")

    dns_zone_id = local.is_warm ? data.terraform_remote_state.shared.outputs.warm_zone.zone_id : data.terraform_remote_state.shared.outputs.hot_zone.zone_id
}
