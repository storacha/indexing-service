package construct

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	dssync "github.com/ipfs/go-datastore/sync"
	flatfs "github.com/ipfs/go-ds-flatfs"
	logging "github.com/ipfs/go-log/v2"
	ipnifind "github.com/ipni/go-libipni/find/client"
	"github.com/ipni/go-libipni/maurl"
	crypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	goredis "github.com/redis/go-redis/v9"
	"github.com/storacha/indexing-service/pkg/internal/jobqueue"
	"github.com/storacha/indexing-service/pkg/redis"
	"github.com/storacha/indexing-service/pkg/service"
	"github.com/storacha/indexing-service/pkg/service/blobindexlookup"
	"github.com/storacha/indexing-service/pkg/service/contentclaims"
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
var contentClaimsNamespace = datastore.NewKey("claims/")

// ServiceConfig sets specific config values for the service
type ServiceConfig struct {

	// PrivateKey configures the private key for the service.
	PrivateKey crypto.PrivKey

	// PublicURL is the public HTTP URL(s) the indexing service is available at.
	// These are used when publishing claims, to indicate that they can be
	// retrieved from here, replacing the pattern "{claim}" with the CID of the
	// claim that is available.
	PublicURL []string

	ProvidersRedis goredis.Options
	ClaimsRedis    goredis.Options
	IndexesRedis   goredis.Options

	// IndexerURL is the URL of an IPNI node to use for find queries.
	IndexerURL string

	// PublisherDirectAnnounceURLs are the URL(s) of IPNI nodes that advertisement
	// announcements should be sent to. Defaults to IndexerURL if not set.
	PublisherDirectAnnounceURLs []string
	// PublisherListenAddr configures the HTTP address the publisher binds to.
	// This allows a remote IPNI subscriber to fetch advertisements.
	PublisherListenAddr string
	// PublisherAnnounceAddrs configures the multiaddrs that are put into announce
	// messages to tell indexers the addresses to fetch advertisements from.
	PublisherAnnounceAddrs []string
}

type config struct {
	cachingQueue     blobindexlookup.CachingQueue
	opts             []service.Option
	ds               datastore.Batching
	skipNotification bool
	startIPNIServer  bool
	publisherStore   store.PublisherStore
	claimsStore      types.ContentClaimsStore
	providersClient  redis.Client
	claimsClient     redis.Client
	indexesClient    redis.Client
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

// WithClaimsStore configures the store used for content claims.
func WithClaimsStore(store types.ContentClaimsStore) Option {
	return func(cfg *config) error {
		cfg.claimsStore = store
		return nil
	}
}

// WithDataPath constructs a flat FS datastore at the specified path to use for
// IPNI advertisements and content claims.
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

// WithDatastore uses the given datastore for storing IPNI advertisements and
// content claims.
func WithDatastore(ds datastore.Batching) Option {
	return func(cfg *config) error {
		cfg.ds = ds
		return nil
	}
}

// WithProvidersClient configures the redis client used for caching providers.
func WithProvidersClient(client redis.Client) Option {
	return func(cfg *config) error {
		cfg.providersClient = client
		return nil
	}
}

// WithClaimsClient configures the redis client used for caching content claims.
func WithClaimsClient(client redis.Client) Option {
	return func(cfg *config) error {
		cfg.claimsClient = client
		return nil
	}
}

// WithIndexesClient configures the redis client used for caching blob indexes.
func WithIndexesClient(client redis.Client) Option {
	return func(cfg *config) error {
		cfg.indexesClient = client
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
	providersClient := cfg.providersClient
	if providersClient == nil {
		providersClient = goredis.NewClient(&sc.ProvidersRedis)
	}
	claimsClient := cfg.claimsClient
	if claimsClient == nil {
		claimsClient = goredis.NewClient(&sc.ClaimsRedis)
	}
	indexesClient := cfg.indexesClient
	if indexesClient == nil {
		indexesClient = goredis.NewClient(&sc.IndexesRedis)
	}

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
		cachingQueue = jq
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

	directAnnounceURLs := sc.PublisherDirectAnnounceURLs
	if len(directAnnounceURLs) == 0 {
		directAnnounceURLs = append(directAnnounceURLs, sc.IndexerURL)
	}

	publisher, err := publisher.New(
		sc.PrivateKey,
		publisherStore,
		publisher.WithDirectAnnounce(directAnnounceURLs...),
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
	providerIndex := providerindex.New(providersCache, findClient, publisher, nil)

	claimsStore := cfg.claimsStore
	if claimsStore == nil {
		if ds == nil {
			ds = initializeDatastore(&cfg)
		}
		claimsStore = contentclaims.NewStoreFromDatastore(namespace.Wrap(ds, contentClaimsNamespace))
	}

	claims := contentclaims.New(claimsStore, claimsCache, http.DefaultClient)
	blobIndexLookup := blobindexlookup.WithCache(
		blobindexlookup.NewBlobIndexLookup(http.DefaultClient),
		shardDagIndexesCache,
		cachingQueue,
	)

	peerID, err := peer.IDFromPrivateKey(sc.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("creating peer ID: %w", err)
	}

	publicAddrInfo := peer.AddrInfo{ID: peerID}
	for _, str := range sc.PublicURL {
		u, err := url.Parse(str)
		if err != nil {
			return nil, fmt.Errorf("parsing public URL: %w", err)
		}
		addr, err := maurl.FromURL(u)
		if err != nil {
			return nil, fmt.Errorf("converting URL to multiaddr: %w", err)
		}
		publicAddrInfo.Addrs = append(publicAddrInfo.Addrs, addr)
	}

	// with concurrency will still get overridden if a different walker setting is used
	serviceOpts := append([]service.Option{service.WithConcurrency(5)}, cfg.opts...)

	s.IndexingService = service.NewIndexingService(blobIndexLookup, claims, publicAddrInfo, providerIndex, serviceOpts...)

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
