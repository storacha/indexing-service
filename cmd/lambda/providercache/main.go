package main

import (
	"context"
	"errors"
	"sync"
	"time"

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

// gracePeriod is the amount of time we have to clean up before a lambda exits
var gracePeriod = time.Second * 3

func main() {
	lambda.Start(makeHandler)
}

func makeHandler(cfg aws.Config) any {
	providersRedis := goredis.NewClusterClient(&cfg.ProvidersRedis)
	if cfg.HoneycombAPIKey != "" {
		providersRedis = telemetry.InstrumentRedisClient(providersRedis)
	}
	providerStore := redis.NewProviderStore(providersRedis)
	providerCacher := providercacher.NewSimpleProviderCacher(providerStore)
	sqsCachingDecoder := aws.NewSQSCachingDecoder(cfg.Config, cfg.CachingBucket)

	return func(ctx context.Context, sqsEvent events.SQSEvent) (events.SQSEventResponse, error) {
		deadline, ok := ctx.Deadline()
		if ok {
			graceDeadline := deadline.Add(-gracePeriod)
			// if graceful shutdown time is after now then we can apply new deadline
			if graceDeadline.After(time.Now()) {
				dctx, cancel := context.WithDeadline(ctx, graceDeadline)
				defer cancel()
				ctx = dctx
			}
		}

		type handlerResult struct {
			id  string
			err error
		}

		// process messages in parallel
		results := make(chan handlerResult, len(sqsEvent.Records))
		var wg sync.WaitGroup
		for _, msg := range sqsEvent.Records {
			wg.Add(1)
			go func(msg events.SQSMessage) {
				defer wg.Done()
				err := handleMessage(ctx, sqsCachingDecoder, providerCacher, msg)
				results <- handlerResult{msg.MessageId, err}
			}(msg)
		}
		wg.Wait()
		// collect errors
		close(results)
		batchItemFailures := []events.SQSBatchItemFailure{}
		var err error
		for r := range results {
			if r.err != nil {
				err = errors.Join(err, r.err)
				batchItemFailures = append(batchItemFailures, events.SQSBatchItemFailure{ItemIdentifier: r.id})
			}
		}
		if err != nil {
			log.Errorf("handling messages: %s", err.Error())
		}
		return events.SQSEventResponse{BatchItemFailures: batchItemFailures}, nil
	}
}

func handleMessage(ctx context.Context, sqsCachingDecoder *aws.SQSCachingDecoder, providerCacher providercacher.ProviderCacher, msg events.SQSMessage) error {
	job, err := sqsCachingDecoder.DecodeMessage(ctx, msg.Body)
	if err != nil {
		return err
	}
	err = providerCacher.CacheProviderForIndexRecords(ctx, job.Provider, job.Index)
	// Do not hold up the queue by re-attempting a cache job that times out. It is
	// probably a big DAG and retrying is unlikely to subsequently succeed.
	if errors.Is(err, context.DeadlineExceeded) {
		log.Warnf("not retrying cache provider job for: %s error: %s", job.Index.Content(), err)
		return nil
	}
	if err != nil {
		return err
	}
	return nil
}
