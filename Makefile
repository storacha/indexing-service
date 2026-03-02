VERSION=$(shell awk -F'"' '/"version":/ {print $$4}' version.json)
COMMIT=$(shell git rev-parse --short HEAD)
DATE=$(shell date -u -Iseconds)
GOFLAGS=-ldflags="-X github.com/storacha/indexing-service/pkg/build.version=$(VERSION)"
DOCKER?=$(shell which docker)

.PHONY: all build clean-indexer test test-nocache ucankey mockery serde indexer-prod indexer-debug docker-setup docker-prod docker-dev

all: build

build: indexer

indexer:
	go build $(GOFLAGS) -o ./indexer ./cmd

clean-indexer:
	rm -f ./indexer

test:
	go test -race -v ./...

test-nocache:
	go clean -testcache && make test

ucangen:
	go build -o ./ucangen cmd/ucangen/main.go

ucankey: ucangen
	./ucangen

mockery:
	mockery --config=.mockery.yaml

pkg/blobindex/datamodel/shardeddagindex_cbor_gen.go: pkg/blobindex/datamodel/shardeddagindex.go
	go run ./scripts/cbor_gen.go

serde: pkg/blobindex/datamodel/shardeddagindex_cbor_gen.go

# Production binary - stripped symbols for smaller size
indexer-prod:
	@echo "Building indexer (production)..."
	go build -ldflags="-s -w -X github.com/storacha/indexing-service/pkg/build.version=$(VERSION)" -o ./indexer ./cmd

# Debug binary - no optimizations, no inlining, full symbols
indexer-debug:
	@echo "Building indexer (debug)..."
	go build -gcflags="all=-N -l" -ldflags="-X github.com/storacha/indexing-service/pkg/build.version=$(VERSION)" -o ./indexer ./cmd

# Docker targets (multi-arch: amd64 + arm64)
docker-setup:
	$(DOCKER) buildx create --name multiarch --use 2>/dev/null || $(DOCKER) buildx use multiarch

docker-prod: docker-setup
	$(DOCKER) buildx build --platform linux/amd64,linux/arm64 --target prod -t indexer:latest .

docker-dev: docker-setup
	$(DOCKER) buildx build --platform linux/amd64,linux/arm64 --target dev -t indexer:dev .
