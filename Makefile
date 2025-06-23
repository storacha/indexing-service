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

.PHONY: mockery
mockery:
	mockery --config=.mockery.yaml

pkg/blobindex/datamodel/shardeddagindex_cbor_gen.go: pkg/blobindex/datamodel/shardeddagindex.go
	go run ./scripts/cbor_gen.go

serde: pkg/blobindex/datamodel/shardeddagindex_cbor_gen.go