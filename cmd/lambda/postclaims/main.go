package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"
	"github.com/honeycombio/otel-config-go/otelconfig"
	ucanserver "github.com/storacha/go-ucanto/server"
	idxconf "github.com/storacha/indexing-service/cmd/config"
	"github.com/storacha/indexing-service/pkg/aws"
	"github.com/storacha/indexing-service/pkg/principalresolver"
	"github.com/storacha/indexing-service/pkg/server"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
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

	// set up OpenTelemetry SDK
	otelShutdown, err := otelconfig.ConfigureOpenTelemetry()
	if err != nil {
		panic(fmt.Errorf("error setting up OpenTelemetry: %s", err))
	}
	defer otelShutdown()

	handler := server.PostClaimsHandler(config.Signer, service, ucanserver.WithPrincipalResolver(presolv.ResolveDIDKey))
	instrumentedHandler := otelhttp.NewHandler(http.HandlerFunc(handler), "PostClaims")
	lambda.Start(httpadapter.NewV2(instrumentedHandler).ProxyWithContext)
}
