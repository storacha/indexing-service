package main

import (
	"context"
	"net/http"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"
	"github.com/storacha/indexing-service/pkg/aws"
	"github.com/storacha/indexing-service/pkg/server"
)

func main() {
	config := aws.FromEnv(context.Background())
	handler := server.GetRootHandler(config.Signer)
	lambda.Start(httpadapter.NewV2(http.HandlerFunc(handler)).ProxyWithContext)
}
