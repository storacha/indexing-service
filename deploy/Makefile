ifneq (,$(wildcard ./.env.terraform))
	include .env.terraform
	export
else
	ifneq ($(DEPLOY_ENV), ci)
  	$(error You haven't setup your .env file. Please refer to the readme)
	endif
endif

ECR_URI=$(TF_VAR_allowed_account_id).dkr.ecr.us-west-2.amazonaws.com
REPLICATED_ECR_URI=$(TF_VAR_allowed_account_id).dkr.ecr.$(TF_VAR_region).amazonaws.com
IMAGE_TAG_BASE=$(ECR_URI)/$(TF_VAR_app)-ecr:$(TF_WORKSPACE)
REPLICATED_IMAGE_TAG_BASE=$(REPLICATED_ECR_URI)/$(TF_VAR_app)-ecr:$(TF_WORKSPACE)

# This is a hack to eval the image tag only when it's a dependency, cause we don't want to set it until docker builds
get_image_tag = IMAGE_TAG = $$(IMAGE_TAG_BASE)-$$(shell docker inspect --format $$(format-filter) $$(IMAGE_TAG_BASE)-latest | sed -e 's/sha256://g')
get_replicated_image_tag = REPLICATED_IMAGE_TAG = $$(REPLICATED_IMAGE_TAG_BASE)-$$(shell docker inspect --format $$(format-filter) $$(IMAGE_TAG_BASE)-latest | sed -e 's/sha256://g')

.PHONY: eval_image_tag

eval_image_tag:
	$(eval $(get_image_tag))
	$(eval $(get_replicated_image_tag))

# GET the absolute location for .env.production.local, again, only after it exists


get_env_file = ENV_FILE = $$(abspath .env.production.local)

.PHONY: eval_env_file
eval_env_file: .env.production.local
	$(eval $(get_env_file))

.PHONY: docker-login
docker-login:
	aws ecr get-login-password --region us-west-2 | docker login --username AWS --password-stdin $(ECR_URI)

.env.terraform:

.env.production.local: .env.terraform .env.production.local.tpl
	./esh -s /bin/bash -o .env.production.local .env.production.local.tpl 

docker-build: docker-login .env.production.local
	docker buildx build --platform linux/arm64 -t $(IMAGE_TAG_BASE)-latest --load ..

format-filter ="{{ .Id }}"

docker-tag: docker-build eval_image_tag
	docker tag $(IMAGE_TAG_BASE)-latest $(IMAGE_TAG)

docker-push: docker-tag eval_image_tag
	docker push $(IMAGE_TAG)

.PHONY: clean-terraform

clean-terraform: eval_image_tag
	tofu -chdir=app destroy -var="image_tag=$(REPLICATED_IMAGE_TAG)"

.PHONY: clean-shared

clean-shared:
	TF_WORKSPACE=default tofu -chdir=shared destroy

.PHONY: clean

clean: clean-terraform clean-shared

app/.terraform:
	TF_WORKSPACE=default tofu -chdir=app init

shared/.terraform:
	TF_WORKSPACE=default tofu -chdir=shared init

.tfworkspace:
	tofu -chdir=app workspace new $(TF_WORKSPACE) 
	touch .tfworkspace

.PHONY: init

init: app/.terraform shared/.terraform .tfworkspace

.PHONY: upgrade-shared

upgrade-shared:
	TF_WORKSPACE=default tofu -chdir=shared init --upgrade

.PHONY: upgrade-app

upgrade-app: upgrade-shared
	tofu -chdir=app init -upgrade

.PHONY: upgrade
upgrade: upgrade-shared upgrade-app

.PHONY: validate-shared

validate-shared: shared/.terraform
	TF_WORKSPACE=default tofu -chdir=shared validate

.PHONY: validate-app

validate-app: app/.terraform .tfworkspace
	tofu -chdir=app validate

.PHONY: validate

validate: validate-shared validate-app

.PHONY: plan-shared

plan-shared: shared/.terraform
	TF_WORKSPACE=default tofu -chdir=shared plan

.PHONY: plan-app

plan-app: app/.terraform .tfworkspace eval_image_tag eval_env_file
	tofu -chdir=app plan -var="image_tag=$(REPLICATED_IMAGE_TAG)" -var='env_files=["$(ENV_FILE)"]'

.PHONY: plan

plan: plan-shared plan-app

ifeq ($(DEPLOY_ENV), ci)
APPLY_ARGS=-input=false --auto-approve
else
APPLY_ARGS=""
endif

.PHONY: apply-shared
apply-shared: shared/.terraform
	TF_WORKSPACE=default tofu -chdir=shared apply $(APPLY_ARGS)

.PHONY: apply-app
apply-app: app/.terraform .tfworkspace docker-push eval_image_tag eval_env_file
	tofu -chdir=app apply -var="image_tag=$(REPLICATED_IMAGE_TAG)" -var='env_files=["$(ENV_FILE)"]' $(APPLY_ARGS)

.PHONY: apply
apply: apply-shared apply-app

.PHONY: console-shared
console-shared: shared/.terraform
	TF_WORKSPACE=default tofu -chdir=shared console

.PHONY: console
console: app/.terraform .tfworkspace eval_image_tag eval_env_file
	tofu -chdir=app console -var="image_tag=$(REPLICATED_IMAGE_TAG)" -var='env_files=["$(ENV_FILE)"]'

.PHONY: wait-deploy
wait-deploy:
	aws deploy wait deployment-successful --region $(TF_VAR_region) --deployment-id $(shell aws deploy list-deployments --region $(TF_VAR_region) --deployment-group-name $(TF_WORKSPACE)-$(TF_VAR_app)-code-deploy-deployment-group --application-name $(TF_WORKSPACE)-$(TF_VAR_app)-code-deploy-app --query "deployments[0]" --output text)

