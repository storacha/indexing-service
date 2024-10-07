package service

import (
	"context"
	"net/http"

	logging "github.com/ipfs/go-log/v2"
	"github.com/ipld/go-ipld-prime/linking"
	ipnifind "github.com/ipni/go-libipni/find/client"
	goredis "github.com/redis/go-redis/v9"
	"github.com/storacha/indexing-service/pkg/internal/jobqueue"
	"github.com/storacha/indexing-service/pkg/redis"
	"github.com/storacha/indexing-service/pkg/service/blobindexlookup"
	"github.com/storacha/indexing-service/pkg/service/claimlookup"
	"github.com/storacha/indexing-service/pkg/service/providercacher"
	"github.com/storacha/indexing-service/pkg/service/providerindex"
)

var log = logging.Logger("service")

type ServiceConfig struct {
	RedisURL    string
	RedisPasswd string
	ProvidersDB int
	ClaimsDB    int
	IndexesDB   int
	IndexerURL  string
}

func Construct(sc ServiceConfig) (*IndexingService, func(context.Context), error) {

	// connect to redis
	providersClient := goredis.NewClient(&goredis.Options{
		Addr:     sc.RedisURL,
		Password: sc.RedisPasswd,
		DB:       sc.ProvidersDB,
	})
	claimsClient := goredis.NewClient(&goredis.Options{
		Addr:     sc.RedisURL,
		Password: sc.RedisPasswd,
		DB:       sc.ClaimsDB,
	})
	indexesClient := goredis.NewClient(&goredis.Options{
		Addr:     sc.RedisURL,
		Password: sc.RedisPasswd,
		DB:       sc.IndexesDB,
	})

	// build caches
	providersCache := redis.NewProviderStore(providersClient)
	claimsCache := redis.NewContentClaimsStore(claimsClient)
	shardDagIndexesCache := redis.NewShardedDagIndexStore(indexesClient)

	// setup and start the provider caching queue for indexes
	cachingJobHandler := providercacher.NewJobHandler(providercacher.NewSimpleProviderCacher(providersCache))
	jobQueue := jobqueue.NewJobQueue(cachingJobHandler.Handle,
		jobqueue.WithBuffer(5),
		jobqueue.WithConcurrency(5),
		jobqueue.WithErrorHandler(func(err error) {
			log.Errorw("caching provider index", "error", err)
		}))
	cachingQueue := providercacher.NewCachingQueue(jobQueue)

	// setup IPNI
	// TODO: switch to double hashed client for reader privacy?
	findClient, err := ipnifind.New(sc.IndexerURL)
	if err != nil {
		return nil, nil, err
	}

	// build read through fetchers
	// TODO: add sender / publisher / linksystem / legacy systems
	providerIndex := providerindex.NewProviderIndex(providersCache, findClient, nil, nil, linking.LinkSystem{}, nil)
	claimLookup := claimlookup.WithCache(claimlookup.NewClaimLookup(http.DefaultClient), claimsCache)
	blobIndexLookup := blobindexlookup.WithCache(
		blobindexlookup.NewBlobIndexLookup(http.DefaultClient),
		shardDagIndexesCache,
		cachingQueue,
	)

	// setup walker
	service := NewIndexingService(blobIndexLookup, claimLookup, providerIndex, WithConcurrency(5))

	// start the job queue
	jobQueue.Startup()

	return service, func(ctx context.Context) {
		jobQueue.Shutdown(ctx)
	}, nil
}
