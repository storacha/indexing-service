package construct

import (
	"context"
	"fmt"
	"net/http"

	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	flatfs "github.com/ipfs/go-ds-flatfs"
	logging "github.com/ipfs/go-log/v2"
	ipnifind "github.com/ipni/go-libipni/find/client"
	crypto "github.com/libp2p/go-libp2p/core/crypto"
	goredis "github.com/redis/go-redis/v9"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/indexing-service/pkg/internal/jobqueue"
	"github.com/storacha/indexing-service/pkg/redis"
	"github.com/storacha/indexing-service/pkg/service"
	"github.com/storacha/indexing-service/pkg/service/blobindexlookup"
	"github.com/storacha/indexing-service/pkg/service/claimlookup"
	"github.com/storacha/indexing-service/pkg/service/providercacher"
	"github.com/storacha/indexing-service/pkg/service/providerindex"
	"github.com/storacha/indexing-service/pkg/service/providerindex/publisher"
	"github.com/storacha/indexing-service/pkg/service/queryresult"
)

var log = logging.Logger("service")
var providerIndexNamespace = datastore.NewKey("providerindex/")
var providerIndexPublisherNamespace = providerIndexNamespace.ChildString("publisher/")

// ServiceConfig sets specific config values for the service
type ServiceConfig struct {

	// PrivateKey configures the private key for the service.
	PrivateKey crypto.PrivKey

	ProvidersRedis goredis.Options
	ClaimsRedis    goredis.Options
	IndexesRedis   goredis.Options

	IndexerURL string

	// PublisherListenAddr configures the HTTP address the publisher binds to.
	// This allows a remote IPNI subscriber to fetch advertisements.
	PublisherListenAddr string
	// PublisherAnnounceAddrs configures the multiaddrs of IPNI nodes that new
	// advertisements should be announced to.
	PublisherAnnounceAddrs []string
}

type config struct {
	cachingQueue     blobindexlookup.CachingQueue
	opts             []service.Option
	ds               datastore.Batching
	skipNotification bool
}

// Option configures how the node is construct
type Option func(*config) error

// WithServiceOptions passes option to the core service
func WithServiceOptions(opts ...service.Option) Option {
	return func(cfg *config) error {
		cfg.opts = opts
		return nil
	}
}

// WithCachingQueue overrides the default caching queue for provider caching
func WithCachingQueue(cachingQueue blobindexlookup.CachingQueue) Option {
	return func(cfg *config) error {
		cfg.cachingQueue = cachingQueue
		return nil
	}
}

// SkipNotification removes setting up IPNI notification handlers
func SkipNotification() Option {
	return func(cfg *config) error {
		cfg.skipNotification = true
		return nil
	}
}

// WithFlatFSData constructs a flat FS datastore at the specified path to use for ads
func WithFlatFSData(dataPath string) Option {
	return func(cfg *config) error {
		fds, err := flatfs.CreateOrOpen(dataPath, flatfs.IPFS_DEF_SHARD, true)
		if err != nil {
			return fmt.Errorf("creating or opening IPNI publisher datastore: %w", err)
		}
		cfg.ds = fds
		return nil
	}
}

// WithDatastore uses the given datastore for storing IPNI adds
func WithDatastore(ds datastore.Batching) Option {
	return func(cfg *config) error {
		cfg.ds = ds
		return nil
	}
}

// Service is the core methods of the indexing service but with additional
type Service interface {
	Startup(ctx context.Context) error
	Shutdown(ctx context.Context) error
	CacheClaim(ctx context.Context, claim delegation.Delegation) error
	PublishClaim(ctx context.Context, claim delegation.Delegation) error
	Query(ctx context.Context, q service.Query) (queryresult.QueryResult, error)
}

type serviceWithLifeCycle struct {
	*service.IndexingService
	startupFuncs  []func(ctx context.Context) error
	shutdownFuncs []func(ctx context.Context) error
}

func (s *serviceWithLifeCycle) Startup(ctx context.Context) error {
	for _, startupFunc := range s.startupFuncs {
		err := startupFunc(ctx)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *serviceWithLifeCycle) Shutdown(ctx context.Context) error {
	for _, shutdownFunc := range s.shutdownFuncs {
		err := shutdownFunc(ctx)
		if err != nil {
			return err
		}
	}
	return nil
}

// Construct constructs a full operational indexing service, using real dependencies
func Construct(sc ServiceConfig, opts ...Option) (Service, error) {
	var cfg config
	for _, opt := range opts {
		err := opt(&cfg)
		if err != nil {
			return nil, err
		}
	}

	s := &serviceWithLifeCycle{}
	// connect to redis
	providersClient := goredis.NewClient(&sc.ProvidersRedis)
	claimsClient := goredis.NewClient(&sc.ClaimsRedis)
	indexesClient := goredis.NewClient(&sc.IndexesRedis)

	// build caches
	providersCache := redis.NewProviderStore(providersClient)
	claimsCache := redis.NewContentClaimsStore(claimsClient)
	shardDagIndexesCache := redis.NewShardedDagIndexStore(indexesClient)

	cachingQueue := cfg.cachingQueue
	if cachingQueue == nil {
		// setup and start the provider caching queue for indexes
		cachingJobHandler := providercacher.NewJobHandler(providercacher.NewSimpleProviderCacher(providersCache))

		jq := jobqueue.NewJobQueue(cachingJobHandler.Handle,
			jobqueue.WithBuffer(5),
			jobqueue.WithConcurrency(5),
			jobqueue.WithErrorHandler(func(err error) {
				log.Errorw("caching provider index", "error", err)
			}))

		s.startupFuncs = append(s.startupFuncs, func(context.Context) error { jq.Startup(); return nil })
		s.shutdownFuncs = append(s.shutdownFuncs, jq.Shutdown)
	}

	// setup IPNI
	// TODO: switch to double hashed client for reader privacy?
	findClient, err := ipnifind.New(sc.IndexerURL)
	if err != nil {
		return nil, err
	}

	ds := cfg.ds
	if ds == nil {
		log.Warnf("datastore not configured, using in-memory store")
		ds = datastore.NewMapDatastore()
	}

	// setup remote sync notification
	var remoteSyncNotifier publisher.RemoteSyncNotifier
	if !cfg.skipNotification {
		notifier, err := publisher.NewNotifierWithStorage(sc.IndexerURL, sc.PrivateKey, namespace.Wrap(ds, providerIndexNamespace))
		if err != nil {
			return nil, fmt.Errorf("creating IPNI remote sync notifier: %w", err)
		}
		s.startupFuncs = append(s.startupFuncs, func(ctx context.Context) error { notifier.Start(ctx); return nil })
		s.shutdownFuncs = append(s.shutdownFuncs, func(context.Context) error { notifier.Stop(); return nil })
		remoteSyncNotifier = notifier
	}

	publisher, err := publisher.New(
		sc.PrivateKey,
		publisher.WithDatastore(namespace.Wrap(ds, providerIndexPublisherNamespace)),
		publisher.WithListenAddr(sc.PublisherListenAddr),
		publisher.WithAnnounceAddr(sc.PublisherAnnounceAddrs...),
	)
	if err != nil {
		return nil, fmt.Errorf("creating IPNI publisher: %w", err)
	}

	s.startupFuncs = append(s.startupFuncs, publisher.Start)
	s.shutdownFuncs = append(s.shutdownFuncs, func(context.Context) error { return publisher.Shutdown() })

	// Setup handling ipni remote sync notifications
	if remoteSyncNotifier != nil {
		remoteSyncNotifier.Notify(providerindex.NewRemoteSyncer(providersCache, publisher).HandleRemoteSync)
	}

	// build read through fetchers
	// TODO: add sender / publisher / linksystem / legacy systems
	providerIndex := providerindex.NewProviderIndex(providersCache, findClient, publisher, nil)
	claimLookup := claimlookup.WithCache(claimlookup.NewClaimLookup(http.DefaultClient), claimsCache)
	blobIndexLookup := blobindexlookup.WithCache(
		blobindexlookup.NewBlobIndexLookup(http.DefaultClient),
		shardDagIndexesCache,
		cachingQueue,
	)

	// with concurrency will still get overridden if a different walker setting is used
	serviceOpts := append([]service.Option{service.WithConcurrency(5)}, cfg.opts...)

	s.IndexingService = service.NewIndexingService(blobIndexLookup, claimLookup, providerIndex, serviceOpts...)

	return s, nil
}
