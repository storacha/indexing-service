package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"
	ucanserver "github.com/storacha/go-ucanto/server"
	idxconf "github.com/storacha/indexing-service/cmd/config"
	"github.com/storacha/indexing-service/pkg/aws"
	"github.com/storacha/indexing-service/pkg/principalresolver"
	"github.com/storacha/indexing-service/pkg/server"
)

func main() {
	config := aws.FromEnv(context.Background())
	service, err := aws.Construct(config)
	if err != nil {
		panic(err)
	}
	presolv, err := principalresolver.New(idxconf.PrincipalMapping)
	if err != nil {
		panic(fmt.Errorf("creating principal resolver: %w", err))
	}
	handler := server.PostClaimsHandler(config.Signer, service, ucanserver.WithPrincipalResolver(presolv.ResolveDIDKey))
	lambda.Start(httpadapter.NewV2(http.HandlerFunc(handler)).ProxyWithContext)
}
