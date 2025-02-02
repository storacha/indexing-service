package lambda

import (
	"context"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/storacha/indexing-service/pkg/aws"
	"github.com/storacha/indexing-service/pkg/telemetry"
)

// handlerFactory is a factory function that returns a function suitable to use as a lambda handler. See
// https://docs.aws.amazon.com/lambda/latest/dg/golang-handler.html#golang-handler-signatures for information on the
// valid signatures a handler function can have to be used as a lambda handler.
type handlerFactory func(cfg aws.Config) any

// Start starts the lambda with the handler obtained from the factory function. makeHandler is a factory function that
// returns a handler suitable to use as a lambda handler.
// The handler is instrumented with OpenTelemetry if a Honeycomb API key is provided.
func Start(makeHandler handlerFactory) {
	ctx := context.Background()
	cfg := aws.FromEnv(ctx)

	// an empty API key disables instrumentation
	if cfg.HoneycombAPIKey != "" {
		telemetryShutdown, err := telemetry.SetupTelemetry(ctx, &cfg.Config)
		if err != nil {
			panic(err)
		}
		defer telemetryShutdown(ctx)

		handler := makeHandler(cfg)
		instrumentedHandler := telemetry.GetInstrumentedLambdaHandler(handler)

		lambda.StartWithOptions(instrumentedHandler, lambda.WithContext(ctx))
	} else {
		lambda.StartWithOptions(makeHandler(cfg), lambda.WithContext(ctx))
	}
}
