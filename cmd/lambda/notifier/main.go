package main

import (
	"context"
	"time"

	"github.com/aws/aws-lambda-go/events"
	logging "github.com/ipfs/go-log/v2"
	"github.com/storacha/indexing-service/cmd/lambda"
	"github.com/storacha/indexing-service/pkg/aws"
	"github.com/storacha/ipni-publisher/pkg/notifier"
)

var log = logging.Logger("lambda/notifier")

func main() {
	lambda.Start(makeHandler)
}

func makeHandler(cfg aws.Config) any {
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
