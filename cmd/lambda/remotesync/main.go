package main

import (
	"context"
	"encoding/json"

	"github.com/aws/aws-lambda-go/events"
	"github.com/ipfs/go-cid"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	goredis "github.com/redis/go-redis/v9"
	"github.com/storacha/go-metadata"
	"github.com/storacha/indexing-service/cmd/lambda"
	"github.com/storacha/indexing-service/pkg/aws"
	"github.com/storacha/indexing-service/pkg/redis"
	"github.com/storacha/indexing-service/pkg/service/providerindex"
	"github.com/storacha/indexing-service/pkg/telemetry"
	"github.com/storacha/ipni-publisher/pkg/store"
)

func main() {
	lambda.Start(makeHandler)
}

func makeHandler(cfg aws.Config) any {
	providersRedis := goredis.NewClient(&cfg.ProvidersRedis)
	if cfg.HoneycombAPIKey != "" {
		providersRedis = telemetry.InstrumentRedisClient(providersRedis)
	}
	providerStore := redis.NewProviderStore(providersRedis)
	ipniStore := aws.NewS3Store(cfg.Config, cfg.IPNIStoreBucket, cfg.IPNIStorePrefix)
	chunkLinksTable := aws.NewDynamoProviderContextTable(cfg.Config, cfg.ChunkLinksTableName)
	metadataTable := aws.NewDynamoProviderContextTable(cfg.Config, cfg.MetadataTableName)
	publisherStore := store.NewPublisherStore(ipniStore, chunkLinksTable, metadataTable, store.WithMetadataContext(metadata.MetadataContext))
	remoteSyncer := providerindex.NewRemoteSyncer(providerStore, publisherStore)

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
