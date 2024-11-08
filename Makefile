ifneq (,$(wildcard ./.env))
	include .env
	export
else
  $(error You haven't setup your .env file. Please refer to the readme)
endif
LAMBDA_GOOS=linux
LAMBDA_GOARCH=arm64
LAMBDA_GOCC?=go
LAMBDA_GOFLAGS=-tags=lambda.norpc
LAMBDA_CGO_ENABLED=0
LAMBDAS=build/getclaim/bootstrap build/getclaims/bootstrap build/getroot/bootstrap build/notifier/bootstrap build/postclaims/bootstrap build/providercache/bootstrap build/remotesync/bootstrap

ucangen:
	go build -o ./ucangen cmd/ucangen/main.go

.PHONY: ucankey

ucankey: ucangen
	./ucangen

.PHONY: clean-lambda

clean-lambda:
	rm -rf build

.PHONY: clean-terraform

clean-terraform:
	tofu -chdir=deploy destroy

.PHONY: clean

clean: clean-lambda clean-terraform

lambdas: $(LAMBDAS)

.PHONY: $(LAMBDAS)

$(LAMBDAS): build/%/bootstrap:
	GOOS=$(LAMBDA_GOOS) GOARCH=$(LAMBDA_GOARCH) CGO_ENABLED=$(LAMBDA_CGO_ENABLED) $(LAMBDA_GOCC) build $(LAMBDA_GOFLAGS) -o $@ cmd/lambda/$*/main.go

deploy/.terraform:
	TF_WORKSPACE= tofu -chdir=deploy init

.tfworkspace:
	tofu -chdir=deploy workspace new $(TF_WORKSPACE)
	touch .tfworkspace

.PHONY: init

init: deploy/.terraform .tfworkspace

.PHONY: validate

validate: deploy/.terraform .tfworkspace
	tofu -chdir=deploy validate

.PHONY: plan

plan: deploy/.terraform .tfworkspace $(LAMBDAS)
	tofu -chdir=deploy plan

.PHONY: apply

apply: deploy/.terraform .tfworkspace $(LAMBDAS)
	tofu -chdir=deploy apply
