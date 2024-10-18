package main

import (
	"context"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	logging "github.com/ipfs/go-log/v2"
	"github.com/storacha/indexing-service/pkg/aws"
	"github.com/storacha/ipni-publisher/pkg/notifier"
)

var log = logging.Logger("lambda/notifier")

func makeHandler(notifier *notifier.Notifier) func(ctx context.Context, event events.EventBridgeEvent) {
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

func main() {
	config := aws.FromEnv(context.Background())
	// setup IPNI
	// TODO: switch to double hashed client for reader privacy?
	headStore := aws.NewS3Store(config.Config, config.NotifierHeadBucket, "")
	notifier, err := notifier.NewNotifierWithStorage(config.IndexerURL, config.PrivateKey, headStore)
	if err != nil {
		panic(err)
	}
	sqsRemoteSyncNotifier := aws.NewSNSRemoteSyncNotifier(config.Config, config.NotifierTopicArn)
	notifier.Notify(sqsRemoteSyncNotifier.NotifyRemoteSync)

	lambda.Start(makeHandler(notifier))
}
