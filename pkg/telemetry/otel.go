package telemetry

import (
	"context"
	"fmt"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
	"github.com/storacha/indexing-service/pkg/construct"
	lambdadetector "go.opentelemetry.io/contrib/detectors/aws/lambda"
	"go.opentelemetry.io/contrib/instrumentation/github.com/aws/aws-lambda-go/otellambda"
	"go.opentelemetry.io/contrib/instrumentation/github.com/aws/aws-sdk-go-v2/otelaws"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/trace"
)

// SetupTelemetry configures the OpenTelemetry SDK by setting up a global tracer provider.
// It also adds instrumentation middleware to the config so that all AWS SDK clients based on that config are instrumented.
// This function updates the configuration in place. It should be called before any AWS SDK clients are created.
func SetupTelemetry(ctx context.Context, cfg *aws.Config) (func(context.Context), error) {
	// WithInsecure is ok here because we are exporting traces to the AWS OpenTelemetry collector which is running
	// in a layer within the lambda's execution environment
	exp, err := otlptracegrpc.New(ctx, otlptracegrpc.WithInsecure())
	if err != nil {
		return nil, err
	}

	// the resource detector populates some span attributes with information about the environment
	detector := lambdadetector.NewResourceDetector()
	resource, err := detector.Detect(ctx)
	if err != nil {
		return nil, err
	}

	tp := trace.NewTracerProvider(
		trace.WithBatcher(exp),
		trace.WithResource(resource),
	)

	shutdownFunc := func(ctx context.Context) {
		err := tp.Shutdown(ctx)
		if err != nil {
			fmt.Printf("error shutting down tracer provider: %v", err)
		}
	}

	// set as the global tracer provider. It is common for OpenTelemetry libraries to use the global tracer provider
	// as the default provider if one is not provided
	otel.SetTracerProvider(tp)

	// instrument all aws clients
	otelaws.AppendMiddlewares(&cfg.APIOptions)

	return shutdownFunc, nil
}

func GetInstrumentedLambdaHandler(handlerFunc interface{}) interface{} {
	tp := otel.GetTracerProvider()
	asFlusher := tp.(otellambda.Flusher)

	return otellambda.InstrumentHandler(
		handlerFunc,
		otellambda.WithTracerProvider(tp),
		otellambda.WithFlusher(asFlusher),
	)
}

func GetInstrumentedHTTPClient() *http.Client {
	httpClient := construct.DefaultHTTPClient()
	instrumentedTransport := otelhttp.NewTransport(httpClient.Transport)
	httpClient.Transport = instrumentedTransport

	return httpClient
}

func GetInstrumentedRedisClient(opts *redis.Options) *redis.Client {
	client := redis.NewClient(opts)
	redisotel.InstrumentTracing(client)
	return client
}
