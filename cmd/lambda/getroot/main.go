package main

import (
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"
	"github.com/storacha/indexing-service/cmd/lambda"
	"github.com/storacha/indexing-service/pkg/aws"
	"github.com/storacha/indexing-service/pkg/server"
)

func main() {
	lambda.Start(makeHandler)
}

func makeHandler(cfg aws.Config) any {
	handler := httpadapter.NewV2(server.GetRootHandler(cfg.Signer)).ProxyWithContext

	return handler
}
