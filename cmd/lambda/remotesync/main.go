package main

import (
	"context"
	"encoding/json"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	goredis "github.com/redis/go-redis/v9"
	"github.com/storacha/indexing-service/pkg/aws"
	"github.com/storacha/indexing-service/pkg/redis"
	"github.com/storacha/indexing-service/pkg/service/providercacher"
	"github.com/storacha/indexing-service/pkg/service/providerindex"
	"github.com/storacha/indexing-service/pkg/service/providerindex/store"
)

var log = logging.Logger("lambda/providercache")

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

func makeHandler(remoteSyncer *providerindex.RemoteSyncer) func(ctx context.Context, snsEvent events.SNSEvent) error {
	return func(ctx context.Context, snsEvent events.SNSEvent) error {
		for _, record := range snsEvent.Records {
			snsRecord := record.SNS
			var snsRemoteSyncMessage aws.SNSRemoteSyncMessage
			err := json.Unmarshal([]byte(snsRecord.Message), &snsRemoteSyncMessage)
			if err != nil {
				return err
			}
			headCid, err := cid.Parse(snsRemoteSyncMessage.Head)
			if err != nil {
				return err
			}
			head := cidlink.Link{Cid: headCid}
			prevCid, err := cid.Parse(snsRemoteSyncMessage.Prev)
			if err != nil {
				return err
			}
			prev := cidlink.Link{Cid: prevCid}
			remoteSyncer.HandleRemoteSync(ctx, head, prev)
		}
		return nil
	}
}

func main() {
	cfg := aws.FromEnv(context.Background())
	providerRedis := goredis.NewClient(&cfg.ProvidersRedis)
	providerStore := redis.NewProviderStore(providerRedis)
	ipniStore := aws.NewS3Store(cfg.Config, cfg.IPNIStoreBucket, cfg.IPNIStorePrefix)
	chunkLinksTable := aws.NewDynamoProviderContextTable(cfg.Config, cfg.ChunkLinksTableName)
	metadataTable := aws.NewDynamoProviderContextTable(cfg.Config, cfg.MetadataTableName)
	publisherStore := store.NewPublisherStore(ipniStore, chunkLinksTable, metadataTable)
	remoteSyncer := providerindex.NewRemoteSyncer(providerStore, publisherStore)
	lambda.Start(makeHandler(remoteSyncer))
}
