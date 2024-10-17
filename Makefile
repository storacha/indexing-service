GOOS=linux
GOARCH=arm64
GOCC?=go
GOFLAGS=-tags=lambda.norpc
CGO_ENABLED=0
LAMBDAS=getclaims getroot notifier postclaims providercache remotesync

lambdas: $(LAMBDAS)

$(LAMBDAS): %:
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=$(CGO_ENABLED) $(GOCC) build $(GOFLAGS) -o build/$@/bootstrap cmd/lambda/$@/main.go
