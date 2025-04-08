ifneq (,$(wildcard ./.env))
	include .env
	export
else
  $(error You haven't setup your .env file. Please refer to the readme)
endif
VERSION=$(shell awk -F'"' '/"version":/ {print $$4}' version.json)
LAMBDA_GOOS=linux
LAMBDA_GOARCH=arm64
LAMBDA_GOCC?=go
LAMBDA_GOFLAGS=-tags=lambda.norpc -ldflags="-s -w -X github.com/storacha/indexing-service/pkg/build.version=$(VERSION)"
LAMBDA_CGO_ENABLED=0
LAMBDADIRS=build/getclaim build/getclaims build/getroot build/notifier build/postclaims build/providercache build/remotesync
LAMBDAS=$(foreach dir, $(LAMBDADIRS), $(dir)/bootstrap)
OTELCOL_CONFIG=otel-collector-config.yaml

indexer:
	go build -o ./indexer ./cmd

.PHONY: clean-indexer

clean-indexer:
	rm -f ./indexer

.PHONY: test

test:
	go test -race -v ./...

.PHONY: test-nocache

test-nocache:
	go clean -testcache && make test

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

lambdas: $(LAMBDAS) otel-config

.PHONY: $(LAMBDAS)

$(LAMBDAS): build/%/bootstrap:
	GOOS=$(LAMBDA_GOOS) GOARCH=$(LAMBDA_GOARCH) CGO_ENABLED=$(LAMBDA_CGO_ENABLED) $(LAMBDA_GOCC) build $(LAMBDA_GOFLAGS) -o $@ ./cmd/lambda/$*

otel-config: otel-collector-config.yaml
	echo $(LAMBDADIRS) | xargs -n 1 cp $(OTELCOL_CONFIG)

deploy/app/.terraform:
	tofu -chdir=deploy/app init

.tfworkspace:
	tofu -chdir=deploy/app workspace new $(TF_WORKSPACE)
	touch .tfworkspace

.PHONY: init

init: deploy/app/.terraform .tfworkspace

.PHONY: upgrade

upgrade:
	tofu -chdir=deploy/app init -upgrade

.PHONY: validate

validate: deploy/app/.terraform .tfworkspace
	tofu -chdir=deploy/app validate

.PHONY: plan

plan: deploy/app/.terraform .tfworkspace lambdas
	tofu -chdir=deploy/app plan

.PHONY: apply

apply: deploy/app/.terraform .tfworkspace lambdas
	tofu -chdir=deploy/app apply

.PHONY: mockery

mockery:
	mockery --config=.mockery.yaml

pkg/blobindex/datamodel/shardeddagindex_cbor_gen.go: pkg/blobindex/datamodel/shardeddagindex.go
	go run ./scripts/cbor_gen.go

serde: pkg/blobindex/datamodel/shardeddagindex_cbor_gen.go