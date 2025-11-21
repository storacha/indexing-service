package telemetry

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
	ecsdetector "go.opentelemetry.io/contrib/detectors/aws/ecs"
	"go.opentelemetry.io/contrib/instrumentation/github.com/aws/aws-lambda-go/otellambda"
	"go.opentelemetry.io/contrib/instrumentation/github.com/aws/aws-sdk-go-v2/otelaws"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

type config struct {
	baseSampler tracesdk.Sampler
}

type TelemetryOption func(*config) error

func WithBaseSampler(baseSampler tracesdk.Sampler) TelemetryOption {
	return func(c *config) error {
		c.baseSampler = baseSampler
		return nil
	}
}

// SetupTelemetry configures the OpenTelemetry SDK by setting up a global tracer provider.
// It also adds instrumentation middleware to the config so that all AWS SDK clients based on that config are instrumented.
// This function updates the configuration in place. It should be called before any AWS SDK clients are created.
func SetupTelemetry(ctx context.Context, cfg *aws.Config, opts ...TelemetryOption) (func(context.Context), error) {
	c := config{
		// Default to only tracing when there is an incoming sampled parent (e.g. upstream service already tracing).
		// This avoids generating root spans for ad-hoc Lambda invocations that arrive without trace headers.
		baseSampler: tracesdk.NeverSample(),
	}
	for _, opt := range opts {
		if err := opt(&c); err != nil {
			return nil, err
		}
	}

	// The default HTTP exporter will fetch the endpoint from the OTEL_EXPORTER_OTLP_ENDPOINT environment variable
	// and will use the headers from the OTEL_EXPORTER_OTLP_HEADERS environment variable
	exp, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, err
	}

	// the resource detector populates some span attributes with information about the environment
	detector := ecsdetector.NewResourceDetector()
	resource, err := detector.Detect(ctx)
	if err != nil {
		return nil, err
	}

	// accept incoming trace context from upstream services in both W3C Trace Context and Baggage formats
	prop := propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
	otel.SetTextMapPropagator(prop)

	// Use ParentBased on the configured base sampler. With the default NeverSample base sampler, we emit spans only
	// when the incoming request already has a sampled parent trace.
	tp := tracesdk.NewTracerProvider(
		tracesdk.WithSampler(tracesdk.ParentBased(c.baseSampler)),
		tracesdk.WithBatcher(exp),
		tracesdk.WithResource(resource),
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

func InstrumentLambdaHandler(handlerFunc interface{}) interface{} {
	tp := otel.GetTracerProvider()
	asFlusher := tp.(otellambda.Flusher)

	return otellambda.InstrumentHandler(
		handlerFunc,
		otellambda.WithEventToCarrier(jsonEventHeadersToCarrier),
		otellambda.WithTracerProvider(tp),
		otellambda.WithFlusher(asFlusher),
	)
}

// jsonEventHeadersToCarrier returns a TextMapCarrier that extracts its data from the passed JSON event. It will look
// for the "headers" field in the event, which is populated by the AWS API Gateway. Therefore, this function will make
// distributed tracing work only with HTTP-based lambdas that are triggered by the API Gateway.
func jsonEventHeadersToCarrier(eventJSON []byte) propagation.TextMapCarrier {
	var apiGatewayEvent struct {
		Headers map[string]string `json:"headers"`
	}

	if err := json.Unmarshal(eventJSON, &apiGatewayEvent); err != nil {
		return propagation.MapCarrier{}
	}

	return propagation.MapCarrier(apiGatewayEvent.Headers)
}

func InstrumentHTTPClient(client *http.Client) *http.Client {
	instrumentedTransport := otelhttp.NewTransport(client.Transport)
	client.Transport = instrumentedTransport

	return client
}

func InstrumentRedisClient(client *redis.ClusterClient) *redis.ClusterClient {
	redisotel.InstrumentTracing(client)
	return client
}

func StartSpan(ctx context.Context, name string) (context.Context, trace.Span) {
	t := otel.Tracer("")
	return t.Start(ctx, name)
}

func Error(span trace.Span, err error, msg string) {
	span.SetStatus(codes.Error, msg)
	span.RecordError(err)
}
