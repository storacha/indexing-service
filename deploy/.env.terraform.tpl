# copy to .env.terraform and set missing vars
TF_WORKSPACE= #your name here
TF_VAR_network= # optional, if the deployments targets a network different from the default specify it here
TF_VAR_app=indexer
TF_VAR_did= # did for your env
TF_VAR_private_key= # private_key or your env -- do not commit to repo!
TF_VAR_allowed_account_id=505595374361
TF_VAR_region=us-west-2