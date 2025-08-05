# copy to .env.terraform and set missing vars
TF_WORKSPACE= # your name here
TF_VAR_network= # optional, if the deployments targets a network different from the default specify it here
TF_VAR_app=indexer
TF_VAR_did= # did for your env
TF_VAR_private_key= # private_key or your env -- do not commit to repo!
TF_VAR_allowed_account_id=505595374361
TF_VAR_region=us-west-2
TF_VAR_cloudflare_zone_id=37783d6f032b78cd97ce37ab6fd42848
CLOUDFLARE_API_TOKEN= # enter a cloudflare api token
HONEYCOMB_API_KEY= # optional - if you want to enable instrumentation (only Honeycomb is supported for now)
SENTRY_DSN= # optional - Sentry DSN for error reporting. Obtain from sentry.io. Leave blank to disable error reporting.
SENTRY_ENVIRONMENT= # optional - Sentry environment to use for error reporting. Defaults to the terraform workspace being used if not set.
