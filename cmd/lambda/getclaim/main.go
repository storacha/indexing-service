package main

import (
	"context"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"
	"github.com/storacha/indexing-service/pkg/aws"
	"github.com/storacha/indexing-service/pkg/server"
	"github.com/storacha/indexing-service/pkg/telemetry"
	"go.opentelemetry.io/contrib/instrumentation/github.com/aws/aws-lambda-go/otellambda"
)

func main() {
	cfg := aws.FromEnv(context.Background())

	// an empty API key disables instrumentation
	if cfg.HoneycombAPIKey != "" {
		ctx := context.Background()
		tp, telemetryShutdown, err := telemetry.SetupTelemetry(ctx, cfg)
		if err != nil {
			panic(err)
		}
		defer telemetryShutdown(ctx)

		handler := makeHandler(cfg)

		instrumentedHandler := otellambda.InstrumentHandler(
			handler,
			otellambda.WithTracerProvider(tp),
			otellambda.WithFlusher(tp),
		)
		lambda.Start(instrumentedHandler)
	} else {
		lambda.Start(makeHandler(cfg))
	}
}

func makeHandler(cfg aws.Config) func(ctx context.Context, event events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	service, err := aws.Construct(cfg)
	if err != nil {
		panic(err)
	}

	handler := httpadapter.NewV2(server.GetClaimHandler(service)).ProxyWithContext

	return handler
}
