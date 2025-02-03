package main

import (
	"fmt"

	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"
	ucanserver "github.com/storacha/go-ucanto/server"
	idxconf "github.com/storacha/indexing-service/cmd/config"
	"github.com/storacha/indexing-service/cmd/lambda"
	"github.com/storacha/indexing-service/pkg/aws"
	"github.com/storacha/indexing-service/pkg/principalresolver"
	"github.com/storacha/indexing-service/pkg/server"
)

func main() {
	lambda.Start(makeHandler)
}

func makeHandler(cfg aws.Config) any {
	service, err := aws.Construct(cfg)
	if err != nil {
		panic(err)
	}

	presolv, err := principalresolver.New(idxconf.PrincipalMapping)
	if err != nil {
		panic(fmt.Errorf("creating principal resolver: %w", err))
	}

	handler := httpadapter.NewV2(server.PostClaimsHandler(cfg.Signer, service, ucanserver.WithPrincipalResolver(presolv.ResolveDIDKey))).ProxyWithContext

	return handler
}
