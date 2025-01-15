package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"
	"github.com/honeycombio/otel-config-go/otelconfig"
	"github.com/storacha/indexing-service/pkg/aws"
	"github.com/storacha/indexing-service/pkg/server"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func main() {
	config := aws.FromEnv(context.Background())

	// set up OpenTelemetry SDK
	otelShutdown, err := otelconfig.ConfigureOpenTelemetry()
	if err != nil {
		panic(fmt.Errorf("error setting up OpenTelemetry: %s", err))
	}
	defer otelShutdown()

	handler := server.GetRootHandler(config.Signer)
	instrumentedHandler := otelhttp.NewHandler(http.HandlerFunc(handler), "GetRoot")
	lambda.Start(httpadapter.NewV2(instrumentedHandler).ProxyWithContext)
}
