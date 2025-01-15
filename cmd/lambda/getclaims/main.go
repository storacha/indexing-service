package main

import (
	"context"
	"fmt"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"
	"github.com/honeycombio/otel-config-go/otelconfig"
	"github.com/storacha/indexing-service/pkg/aws"
	"github.com/storacha/indexing-service/pkg/server"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func main() {
	config := aws.FromEnv(context.Background())
	service, err := aws.Construct(config)
	if err != nil {
		panic(err)
	}

	handler := server.GetClaimsHandler(service)

	// an empty API key disables instrumentation
	if config.HoneycombAPIKey != "" {
		headers := map[string]string{"x-honeycomb-team": config.HoneycombAPIKey}
		otelShutdown, err := otelconfig.ConfigureOpenTelemetry(otelconfig.WithHeaders(headers))
		if err != nil {
			panic(fmt.Errorf("error setting up OpenTelemetry: %s", err))
		}
		defer otelShutdown()

		instrumentedHandler := otelhttp.NewHandler(handler, "GetClaims")
		lambda.Start(httpadapter.NewV2(instrumentedHandler).ProxyWithContext)
	} else {
		lambda.Start(httpadapter.NewV2(handler).ProxyWithContext)
	}
}
