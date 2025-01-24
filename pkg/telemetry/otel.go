package telemetry

import (
	"context"
	"fmt"

	"github.com/storacha/indexing-service/pkg/aws"
	lambdadetector "go.opentelemetry.io/contrib/detectors/aws/lambda"
	"go.opentelemetry.io/contrib/instrumentation/github.com/aws/aws-sdk-go-v2/otelaws"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/trace"
)

func SetupTelemetry(ctx context.Context, config aws.Config) (*trace.TracerProvider, func(context.Context), error) {
	// WithInsecure is ok here because we are exporting traces to the AWS OpenTelemetry collector which is running
	// in a layer within the lambda's execution environment
	exp, err := otlptracegrpc.New(ctx, otlptracegrpc.WithInsecure())
	if err != nil {
		return nil, nil, err
	}

	// the resource detector populates some span attributes with information about the environment
	detector := lambdadetector.NewResourceDetector()
	resource, err := detector.Detect(ctx)
	if err != nil {
		return nil, nil, err
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

	// instrument all aws clients
	otelaws.AppendMiddlewares(&config.APIOptions, otelaws.WithTracerProvider(tp))

	// set as the global tracer provider
	otel.SetTracerProvider(tp)

	return tp, shutdownFunc, nil
}
