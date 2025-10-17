package main

import (
	"fmt"

	goredis "github.com/redis/go-redis/v9"
	"github.com/storacha/go-libstoracha/ipnipublisher/notifier"
	"github.com/storacha/go-libstoracha/ipnipublisher/publisher"
	awspublisherqueue "github.com/storacha/go-libstoracha/ipnipublisher/queue/aws"
	"github.com/storacha/go-libstoracha/ipnipublisher/store"
	"github.com/storacha/go-libstoracha/metadata"
	userver "github.com/storacha/go-ucanto/server"
	"github.com/storacha/indexing-service/pkg/aws"
	"github.com/storacha/indexing-service/pkg/principalresolver"
	"github.com/storacha/indexing-service/pkg/redis"
	"github.com/storacha/indexing-service/pkg/server"
	"github.com/storacha/indexing-service/pkg/service/providercacher"
	"github.com/storacha/indexing-service/pkg/service/providerindex/remotesyncer"
	"github.com/storacha/indexing-service/pkg/service/publishingqueue"
	"github.com/storacha/indexing-service/pkg/telemetry"
	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel/sdk/trace"
)

var awsCmd = &cli.Command{
	Name:  "aws",
	Usage: "Run the indexing service as a containerized server in AWS",
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:    "port",
			Aliases: []string{"p"},
			Value:   8080,
			Usage:   "Port to bind the server to",
		},
	},
	Action: func(cCtx *cli.Context) error {
		addr := fmt.Sprintf(":%d", cCtx.Int("port"))
		cfg := aws.FromEnv(cCtx.Context)
		srvOpts := []server.Option{
			server.WithIdentity(cfg.Signer),
		}

		presolv, err := principalresolver.New(cfg.PrincipalMapping)
		if err != nil {
			return fmt.Errorf("creating principal resolver: %w", err)
		}

		srvOpts = append(
			srvOpts,
			server.WithContentClaimsOptions(
				userver.WithPrincipalResolver(presolv.ResolveDIDKey),
			),
		)

		ipniSrvOpts, err := ipniOpts(cfg.IPNIFormatPeerID, cfg.IPNIFormatEndpoint)
		if err != nil {
			return fmt.Errorf("setting up IPNI options: %w", err)
		}
		srvOpts = append(srvOpts, ipniSrvOpts...)
		// an empty API key disables instrumentation
		if cfg.HoneycombAPIKey != "" {
			var telemetryOpts []telemetry.TelemetryOption
			if cfg.BaseTraceSampleRatio < 1.0 {
				telemetryOpts = append(telemetryOpts, telemetry.WithBaseSampler(trace.TraceIDRatioBased(cfg.BaseTraceSampleRatio)))
			}
			telemetryShutdown, err := telemetry.SetupTelemetry(cCtx.Context, &cfg.Config, telemetryOpts...)
			if err != nil {
				panic(err)
			}
			defer telemetryShutdown(cCtx.Context)

			srvOpts = append(srvOpts, server.WithTelemetry())
		}

		indexer, err := aws.Construct(cfg)
		if err != nil {
			return err
		}

		notifier, err := setupIPNIPipeline(cfg)
		if err != nil {
			return err
		}
		notifier.Start(cCtx.Context)
		defer notifier.Stop()

		cacher, err := setupProviderCacher(cfg)
		if err != nil {
			return err
		}
		cacher.Start()
		defer cacher.Stop()

		publisher, err := setupIPNIPublisher(cfg)
		if err != nil {
			return err
		}
		publisher.Start()
		defer publisher.Stop()

		srvOpts = append(srvOpts, server.WithIPNIPublisherStore(setupIPNIPublisherStore(cfg)))

		return server.ListenAndServe(addr, indexer, srvOpts...)
	},
}

func setupIPNIPipeline(cfg aws.Config) (*notifier.Notifier, error) {
	// setup remote IPNI syncer
	providersRedis := goredis.NewClusterClient(&cfg.ProvidersRedis)
	if cfg.HoneycombAPIKey != "" {
		providersRedis = telemetry.InstrumentRedisClient(providersRedis)
	}
	providerStore := redis.NewProviderStore(redis.NewClusterClientAdapter(providersRedis))
	publisherStore := setupIPNIPublisherStore(cfg)
	remoteSyncer := remotesyncer.New(providerStore, publisherStore)

	// setup notifier to periodically check IPNI and notify remote syncer if updates are required
	headStore := aws.NewS3Store(cfg.Config, cfg.NotifierHeadBucket, "")
	notifier, err := notifier.NewNotifierWithStorage(cfg.IPNIFindURL, cfg.PrivateKey, headStore)
	if err != nil {
		return nil, fmt.Errorf("creating notifier: %w", err)
	}

	notifier.Notify(remoteSyncer.HandleRemoteSync)
	return notifier, nil
}

func setupProviderCacher(cfg aws.Config) (*providercacher.CachingQueuePoller, error) {
	cachingQueue := aws.NewSQSCachingQueue(cfg.Config, cfg.SQSCachingQueueID, cfg.CachingBucket)

	providersRedis := goredis.NewClusterClient(&cfg.ProvidersRedis)
	if cfg.HoneycombAPIKey != "" {
		providersRedis = telemetry.InstrumentRedisClient(providersRedis)
	}
	providerStore := redis.NewProviderStore(redis.NewClusterClientAdapter(providersRedis))
	providerCacher := providercacher.NewSimpleProviderCacher(providerStore)

	return providercacher.NewCachingQueuePoller(cachingQueue, providerCacher)
}

func setupIPNIPublisherStore(cfg aws.Config) *store.AdStore {
	ipniStore := aws.NewS3Store(cfg.Config, cfg.IPNIStoreBucket, cfg.IPNIStorePrefix)
	chunkLinksTable := aws.NewDynamoProviderContextTable(cfg.Config, cfg.ChunkLinksTableName)
	metadataTable := aws.NewDynamoProviderContextTable(cfg.Config, cfg.MetadataTableName)
	return store.NewPublisherStore(ipniStore, chunkLinksTable, metadataTable, store.WithMetadataContext(metadata.MetadataContext))
}

func setupIPNIPublisher(cfg aws.Config) (*publishingqueue.PublishingQueuePoller, error) {
	publisherQueue := awspublisherqueue.NewSQSPublishingQueue(cfg.Config, cfg.SQSPublishingQueueID, cfg.PublishingBucket)
	publisher, err := publisher.New(
		cfg.ServiceConfig.PrivateKey,
		setupIPNIPublisherStore(cfg),
		publisher.WithDirectAnnounce(cfg.ServiceConfig.IPNIDirectAnnounceURLs...),
		publisher.WithAnnounceAddrs(cfg.ServiceConfig.IPNIAnnounceAddrs...),
	)
	if err != nil {
		return nil, fmt.Errorf("creating IPNI publisher: %w", err)
	}
	return publishingqueue.NewPublishingQueuePoller(publisherQueue, publisher)
}
