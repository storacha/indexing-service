package main

import (
	"context"
	"errors"
	"sync"

	"github.com/aws/aws-lambda-go/events"
	logging "github.com/ipfs/go-log/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/storacha/indexing-service/cmd/lambda"
	"github.com/storacha/indexing-service/pkg/aws"
	"github.com/storacha/indexing-service/pkg/redis"
	"github.com/storacha/indexing-service/pkg/service/providercacher"
	"github.com/storacha/indexing-service/pkg/telemetry"
)

var log = logging.Logger("lambda/providercache")

func main() {
	lambda.Start(makeHandler)
}

func makeHandler(cfg aws.Config) any {
	var providersRedis *goredis.Client
	if cfg.HoneycombAPIKey != "" {
		providersRedis = telemetry.GetInstrumentedRedisClient(&cfg.ProvidersRedis)
	} else {
		providersRedis = goredis.NewClient(&cfg.ProvidersRedis)
	}
	providerStore := redis.NewProviderStore(providersRedis)
	providerCacher := providercacher.NewSimpleProviderCacher(providerStore)
	sqsCachingDecoder := aws.NewSQSCachingDecoder(cfg.Config, cfg.CachingBucket)

	return func(ctx context.Context, sqsEvent events.SQSEvent) error {
		// process messages in parallel
		results := make(chan error, len(sqsEvent.Records))
		var wg sync.WaitGroup
		for _, msg := range sqsEvent.Records {
			wg.Add(1)
			go func(msg events.SQSMessage) {
				defer wg.Done()
				err := handleMessage(ctx, sqsCachingDecoder, providerCacher, msg)
				results <- err
			}(msg)
		}
		wg.Wait()
		// collect errors
		close(results)
		var err error
		for nextErr := range results {
			err = errors.Join(err, nextErr)
		}
		// return overall error
		if err != nil {
			return err
		}
		for _, msg := range sqsEvent.Records {
			err := sqsCachingDecoder.CleanupMessage(ctx, msg.Body)
			if err != nil {
				log.Warnf("unable to cleanup message fully: %s", err.Error())
			}
		}
		return nil
	}
}

func handleMessage(ctx context.Context, sqsCachingDecoder *aws.SQSCachingDecoder, providerCacher providercacher.ProviderCacher, msg events.SQSMessage) error {
	job, err := sqsCachingDecoder.DecodeMessage(ctx, msg.Body)
	if err != nil {
		return err
	}
	_, err = providerCacher.CacheProviderForIndexRecords(ctx, job.Provider, job.Index)
	if err != nil {
		return err
	}
	return nil
}
