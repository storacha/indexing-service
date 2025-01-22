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

indexer:
	go build -o ./indexer ./cmd

.PHONY: clean-indexer

clean-indexer:
	rm -f ./indexer

.PHONY: test

test:
	go clean -testcache && go test -race -v ./...

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
	tofu -chdir=deploy/app destroy

.PHONY: clean

clean: clean-terraform clean-lambda clean-indexer

lambdas: $(LAMBDAS)

.PHONY: $(LAMBDAS)

$(LAMBDAS): build/%/bootstrap:
	GOOS=$(LAMBDA_GOOS) GOARCH=$(LAMBDA_GOARCH) CGO_ENABLED=$(LAMBDA_CGO_ENABLED) $(LAMBDA_GOCC) build $(LAMBDA_GOFLAGS) -o $@ cmd/lambda/$*/main.go

deploy/app/.terraform:
	tofu -chdir=deploy/app init

.tfworkspace:
	tofu -chdir=deploy/app workspace new $(TF_WORKSPACE)
	touch .tfworkspace

.PHONY: init

init: deploy/app/.terraform .tfworkspace

.PHONY: validate

validate: deploy/app/.terraform .tfworkspace
	tofu -chdir=deploy/app validate

.PHONY: plan

plan: deploy/app/.terraform .tfworkspace $(LAMBDAS)
	tofu -chdir=deploy/app plan

.PHONY: apply

apply: deploy/app/.terraform .tfworkspace $(LAMBDAS)
	tofu -chdir=deploy/app apply

.PHONY: mockery

mockery:
	mockery --config=.mockery.yaml
