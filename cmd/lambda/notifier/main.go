package main

import (
	"context"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	logging "github.com/ipfs/go-log/v2"
	"github.com/storacha/indexing-service/pkg/aws"
	"github.com/storacha/indexing-service/pkg/telemetry"
	"github.com/storacha/ipni-publisher/pkg/notifier"
	"go.opentelemetry.io/contrib/instrumentation/github.com/aws/aws-lambda-go/otellambda"
)

var log = logging.Logger("lambda/notifier")

func main() {
	cfg := aws.FromEnv(context.Background())

	// an empty API key disables instrumentation
	if cfg.HoneycombAPIKey != "" {
		ctx := context.Background()
		tp, telemetryShutdown, err := telemetry.SetupTelemetry(ctx, &cfg)
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
		lambda.StartWithOptions(instrumentedHandler, lambda.WithContext(ctx))
	} else {
		lambda.Start(makeHandler(cfg))
	}
}

func makeHandler(cfg aws.Config) func(ctx context.Context, event events.EventBridgeEvent) {
	// setup IPNI
	// TODO: switch to double hashed client for reader privacy?
	headStore := aws.NewS3Store(cfg.Config, cfg.NotifierHeadBucket, "")
	notifier, err := notifier.NewNotifierWithStorage(cfg.IndexerURL, cfg.PrivateKey, headStore)
	if err != nil {
		panic(err)
	}
	sqsRemoteSyncNotifier := aws.NewSNSRemoteSyncNotifier(cfg.Config, cfg.NotifierTopicArn)
	notifier.Notify(sqsRemoteSyncNotifier.NotifyRemoteSync)

	return func(ctx context.Context, event events.EventBridgeEvent) {
		synced, ts, err := notifier.Update(ctx)
		if err != nil {
			log.Errorf("error during notifier sync head check: %s", err.Error())
			return
		}
		if !synced {
			log.Warnf("remote IPNI subscriber did not sync for %s", time.Since(ts))
		}
	}
}
