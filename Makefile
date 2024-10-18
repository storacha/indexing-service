ifneq (,$(wildcard ./.env))
	include .env
	export
else
  $(error You haven't setup your .env file. Please refer to the readme)
endif
GOOS=linux
GOARCH=arm64
GOCC?=go
GOFLAGS=-tags=lambda.norpc
CGO_ENABLED=0
LAMBDAS=build/getclaims/bootstrap build/getroot/bootstrap build/notifier/bootstrap build/postclaims/bootstrap build/providercache/bootstrap build/remotesync/bootstrap

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
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=$(CGO_ENABLED) $(GOCC) build $(GOFLAGS) -o $@ cmd/lambda/$*/main.go

deploy/.terraform:
	tofu -chdir=deploy init

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
