locals {
    is_production = terraform.workspace == "prod" || terraform.workspace == "warm-prod"
    is_staging = terraform.workspace == "staging" || terraform.workspace == "warm-staging"
}
