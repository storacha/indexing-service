package telemetry

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
)

// SetupClientTelemetry installs a minimal tracer provider for client-side usage so spans have valid IDs
// and trace context can be propagated. It uses an OTLP/HTTP exporter if OTEL_EXPORTER_OTLP_ENDPOINT is set;
// otherwise it runs with no exporter (noop span export) to avoid failing in CLI contexts.
func SetupClientTelemetry(ctx context.Context) (func(context.Context) error, error) {
	var opts []tracesdk.TracerProviderOption

	// default to AlwaysSample for client spans so manual calls can create traces even without a parent
	opts = append(opts, tracesdk.WithSampler(tracesdk.AlwaysSample()))

	// try to create an exporter if an endpoint is configured; otherwise fall back to no exporter
	exp, err := otlptracehttp.New(ctx)
	if err == nil {
		opts = append(opts, tracesdk.WithBatcher(exp))
	}

	// even without resource attributes, ensure we have a valid provider
	opts = append(opts, tracesdk.WithResource(sdkresource.Empty()))

	tp := tracesdk.NewTracerProvider(opts...)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp.Shutdown, nil
}
