package construct

import (
	"context"
	"fmt"
	"net/http"

	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	dssync "github.com/ipfs/go-datastore/sync"
	flatfs "github.com/ipfs/go-ds-flatfs"
	logging "github.com/ipfs/go-log/v2"
	ipnifind "github.com/ipni/go-libipni/find/client"
	crypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	goredis "github.com/redis/go-redis/v9"
	"github.com/storacha/indexing-service/pkg/internal/jobqueue"
	"github.com/storacha/indexing-service/pkg/redis"
	"github.com/storacha/indexing-service/pkg/service"
	"github.com/storacha/indexing-service/pkg/service/blobindexlookup"
	"github.com/storacha/indexing-service/pkg/service/claimlookup"
	"github.com/storacha/indexing-service/pkg/service/providercacher"
	"github.com/storacha/indexing-service/pkg/service/providerindex"
	"github.com/storacha/indexing-service/pkg/types"
	"github.com/storacha/ipni-publisher/pkg/notifier"
	"github.com/storacha/ipni-publisher/pkg/publisher"
	"github.com/storacha/ipni-publisher/pkg/server"
	"github.com/storacha/ipni-publisher/pkg/store"
)

var log = logging.Logger("service")
var providerIndexNamespace = datastore.NewKey("providerindex/")
var providerIndexPublisherNamespace = providerIndexNamespace.Child(datastore.NewKey("publisher/"))

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
	startIPNIServer  bool
	publisherStore   store.PublisherStore
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

// WithStartIPNIServer determines when IPNI ads will be served directly over HTTP
// Defaults true
func WithStartIPNIServer(startIPNIServer bool) Option {
	return func(cfg *config) error {
		cfg.startIPNIServer = startIPNIServer
		return nil
	}
}

// WithPublisherStore overrides the store used for IPNI advertisements
// If not used with startIPNIServer = false, store.AdvertStore must implement store.FullStore
func WithPublisherStore(publisherStore store.PublisherStore) Option {
	return func(cfg *config) error {
		cfg.publisherStore = publisherStore
		return nil
	}
}

// WithDataPath constructs a flat FS datastore at the specified path to use for ads
func WithDataPath(dataPath string) Option {
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
// lifecycle methods.
type Service interface {
	types.Service
	Startup(ctx context.Context) error
	Shutdown(ctx context.Context) error
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

	var ds datastore.Batching
	publisherStore := cfg.publisherStore
	if publisherStore == nil {
		ds = initializeDatastore(&cfg)
		// setup the datastore for publishing to IPNI
		publisherStore = store.FromDatastore(namespace.Wrap(ds, providerIndexPublisherNamespace))
	}

	// setup remote sync notification
	if !cfg.skipNotification {
		// initialize datastore if not already initialized
		if ds == nil {
			ds = initializeDatastore(&cfg)
		}
		notifierStore := store.SimpleStoreFromDatastore(namespace.Wrap(ds, providerIndexNamespace))
		notifier, err := notifier.NewNotifierWithStorage(sc.IndexerURL, sc.PrivateKey, notifierStore)
		if err != nil {
			return nil, fmt.Errorf("creating IPNI remote sync notifier: %w", err)
		}
		s.startupFuncs = append(s.startupFuncs, func(ctx context.Context) error { notifier.Start(ctx); return nil })
		s.shutdownFuncs = append(s.shutdownFuncs, func(context.Context) error { notifier.Stop(); return nil })
		// Setup handling ipni remote sync notifications
		notifier.Notify(providerindex.NewRemoteSyncer(providersCache, publisherStore).HandleRemoteSync)
	}

	publisher, err := publisher.New(
		sc.PrivateKey,
		publisherStore,
		publisher.WithDirectAnnounce(sc.IndexerURL),
		publisher.WithAnnounceAddrs(sc.PublisherAnnounceAddrs...),
	)
	if err != nil {
		return nil, fmt.Errorf("creating IPNI publisher: %w", err)
	}

	if cfg.startIPNIServer {
		encodableStore, ok := publisherStore.(store.EncodeableStore)
		if !ok {
			return nil, fmt.Errorf("publisher store is incompatible with serving over HTTP (must implement store.EncodableStore)")
		}
		srv, err := server.NewServer(encodableStore, server.WithHTTPListenAddrs(sc.PublisherListenAddr))
		if err != nil {
			return nil, fmt.Errorf("creating server for IPNI ads: %w", err)
		}
		s.startupFuncs = append(s.startupFuncs, srv.Start)
		s.shutdownFuncs = append(s.shutdownFuncs, srv.Shutdown)
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

	peerID, err := peer.IDFromPrivateKey(sc.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("creating peer ID: %w", err)
	}
	provider := peer.AddrInfo{ID: peerID}

	// with concurrency will still get overridden if a different walker setting is used
	serviceOpts := append([]service.Option{service.WithConcurrency(5)}, cfg.opts...)

	s.IndexingService = service.NewIndexingService(blobIndexLookup, claimLookup, provider, providerIndex, serviceOpts...)

	return s, nil
}

func initializeDatastore(cfg *config) datastore.Batching {
	ds := cfg.ds
	if ds == nil {
		log.Warnf("datastore not configured, using in-memory store")
		ds = dssync.MutexWrap(datastore.NewMapDatastore())
	}
	return ds
}
