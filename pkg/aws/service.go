package aws

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/getsentry/sentry-go"
	logging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p/core/crypto"
	goredis "github.com/redis/go-redis/v9"
	publisherqueue "github.com/storacha/go-libstoracha/ipnipublisher/queue"
	awspublisherqueue "github.com/storacha/go-libstoracha/ipnipublisher/queue/aws"
	"github.com/storacha/go-libstoracha/ipnipublisher/store"
	"github.com/storacha/go-libstoracha/metadata"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/go-ucanto/did"
	"github.com/storacha/go-ucanto/principal"
	ed25519 "github.com/storacha/go-ucanto/principal/ed25519/signer"
	"github.com/storacha/go-ucanto/principal/signer"
	"github.com/storacha/indexing-service/pkg/build"
	"github.com/storacha/indexing-service/pkg/construct"
	"github.com/storacha/indexing-service/pkg/presets"
	"github.com/storacha/indexing-service/pkg/redis"
	"github.com/storacha/indexing-service/pkg/service/contentclaims"
	"github.com/storacha/indexing-service/pkg/service/providerindex/legacy"
	"github.com/storacha/indexing-service/pkg/telemetry"
	"github.com/storacha/indexing-service/pkg/types"
)

// ErrNoPrivateKey means that the value returned from Secrets was empty
var ErrNoPrivateKey = errors.New("no value for private key")

func mustGetEnv(envVar string) string {
	value := os.Getenv(envVar)
	if len(value) == 0 {
		panic(fmt.Errorf("missing env var: %s", envVar))
	}
	return value
}

func mustGetInt(envVar string) int64 {
	stringValue := mustGetEnv(envVar)
	value, err := strconv.ParseInt(stringValue, 10, 64)
	if err != nil {
		panic(fmt.Errorf("parsing env var %s to int: %w", envVar, err))
	}
	return value
}

func mustGetFloat(envVar string) float64 {
	stringValue := mustGetEnv(envVar)
	value, err := strconv.ParseFloat(stringValue, 64)
	if err != nil {
		panic(fmt.Errorf("parsing env var %s to int: %w", envVar, err))
	}
	return value
}

// Config describes all the values required to setup AWS from the environment
type Config struct {
	construct.ServiceConfig
	aws.Config
	ProvidersCacheExpirationSeconds   int64
	NoProvidersCacheExpirationSeconds int64
	ClaimsCacheExpirationSeconds      int64
	IndexesCacheExpirationSeconds     int64
	SQSCachingQueueID                 string
	CachingBucket                     string
	SQSPublishingQueueID              string
	PublishingBucket                  string
	ChunkLinksTableName               string
	MetadataTableName                 string
	IPNIStoreBucket                   string
	IPNIStorePrefix                   string
	IPNIAnnounceURLs                  []url.URL
	NotifierHeadBucket                string
	NotifierTopicArn                  string
	ClaimStoreBucket                  string
	ClaimStorePrefix                  string
	LegacyClaimsTableName             string
	LegacyClaimsTableRegion           string
	LegacyClaimsBucket                string
	LegacyBlockIndexTableName         string
	LegacyBlockIndexTableRegion       string
	LegacyStoreTableName              string
	LegacyStoreTableRegion            string
	LegacyBlobRegistryTableName       string
	LegacyBlobRegistryTableRegion     string
	LegacyAllocationsTableName        string
	LegacyAllocationsTableRegion      string
	LegacyDotStorageBucketPrefixes    []string // legacy .storage buckets
	LegacyDataBucketURL               string
	BaseTraceSampleRatio              float64
	SentryDSN                         string
	SentryEnvironment                 string
	HoneycombAPIKey                   string
	PrincipalMapping                  map[string]string
	IPNIFormatPeerID                  string
	IPNIFormatEndpoint                string
	principal.Signer
}

// FromEnv constructs the AWS Configuration from the environment
func FromEnv(ctx context.Context) Config {
	awsConfig, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		panic(fmt.Errorf("loading aws default config: %w", err))
	}

	id, err := ed25519.Parse(mustGetEnv("PRIVATE_KEY"))
	if err != nil {
		panic(fmt.Errorf("parsing private key: %s", err))
	}

	if len(os.Getenv("DID")) != 0 {
		d, err := did.Parse(os.Getenv("DID"))
		if err != nil {
			panic(fmt.Errorf("parsing DID: %w", err))
		}
		id, err = signer.Wrap(id, d)
		if err != nil {
			panic(fmt.Errorf("wrapping server DID: %w", err))
		}
	}

	cryptoPrivKey, err := crypto.UnmarshalEd25519PrivateKey(id.Raw())
	if err != nil {
		panic(fmt.Errorf("unmarshaling private key: %w", err))
	}

	ipniStoreKeyPrefix := os.Getenv("IPNI_STORE_KEY_PREFIX")
	if len(ipniStoreKeyPrefix) == 0 {
		ipniStoreKeyPrefix = "ipni/v1/ad/"
	}

	ipniPublisherAnnounceAddress := fmt.Sprintf("/dns/%s/https", mustGetEnv("IPNI_STORE_BUCKET_REGIONAL_DOMAIN"))

	var principalMapping map[string]string
	if os.Getenv("PRINCIPAL_MAPPING") != "" {
		principalMapping = map[string]string{}
		maps.Copy(principalMapping, presets.PrincipalMapping)
		var pm map[string]string
		err := json.Unmarshal([]byte(os.Getenv("PRINCIPAL_MAPPING")), &pm)
		if err != nil {
			panic(fmt.Errorf("parsing principal mapping: %w", err))
		}
		maps.Copy(principalMapping, pm)
	} else {
		principalMapping = presets.PrincipalMapping
	}

	ipniFindURL := os.Getenv("IPNI_ENDPOINT")
	if ipniFindURL == "" {
		ipniFindURL = presets.IPNIFindURL
	}

	var ipniPublisherDirectAnnounceURLs []string
	if os.Getenv("IPNI_ANNOUNCE_URLS") != "" {
		err := json.Unmarshal([]byte(os.Getenv("IPNI_ANNOUNCE_URLS")), &ipniPublisherDirectAnnounceURLs)
		if err != nil {
			panic(fmt.Errorf("parsing IPNI announce URLs JSON: %w", err))
		}
	} else {
		ipniPublisherDirectAnnounceURLs = presets.IPNIAnnounceURLs
	}

	var legacyDotStorageBucketPrefixes []string
	err = json.Unmarshal([]byte(mustGetEnv("LEGACY_DOT_STORAGE_BUCKET_PREFIXES")), &legacyDotStorageBucketPrefixes)
	if err != nil {
		panic(fmt.Errorf("parsing legacy dot storage bucket prefixes JSON: %w", err))
	}

	return Config{
		Config: awsConfig,
		Signer: id,
		ServiceConfig: construct.ServiceConfig{
			PrivateKey: cryptoPrivKey,
			PublicURL:  strings.Split(mustGetEnv("PUBLIC_URL"), ","),
			ProvidersRedis: goredis.ClusterOptions{
				Addrs:                      []string{mustGetEnv("PROVIDERS_CACHE_URL")},
				ReadOnly:                   true,
				RouteRandomly:              true,
				CredentialsProviderContext: redisCredentialVerifier(awsConfig, mustGetEnv("CACHE_USER_ID"), mustGetEnv("PROVIDERS_CACHE_ID")),
				TLSConfig: &tls.Config{
					MinVersion: tls.VersionTLS12,
				},
			},
			NoProviderRedis: goredis.ClusterOptions{
				Addrs:                      []string{mustGetEnv("NO_PROVIDERS_CACHE_URL")},
				ReadOnly:                   true,
				RouteRandomly:              true,
				CredentialsProviderContext: redisCredentialVerifier(awsConfig, mustGetEnv("CACHE_USER_ID"), mustGetEnv("NO_PROVIDERS_CACHE_ID")),
				TLSConfig: &tls.Config{
					MinVersion: tls.VersionTLS12,
				},
			},
			ClaimsRedis: goredis.ClusterOptions{
				Addrs:                      []string{mustGetEnv("CLAIMS_CACHE_URL")},
				ReadOnly:                   true,
				RouteRandomly:              true,
				CredentialsProviderContext: redisCredentialVerifier(awsConfig, mustGetEnv("CACHE_USER_ID"), mustGetEnv("CLAIMS_CACHE_ID")),
				TLSConfig: &tls.Config{
					MinVersion: tls.VersionTLS12,
				},
			},
			IndexesRedis: goredis.ClusterOptions{
				Addrs:                      []string{mustGetEnv("INDEXES_CACHE_URL")},
				ReadOnly:                   true,
				RouteRandomly:              true,
				CredentialsProviderContext: redisCredentialVerifier(awsConfig, mustGetEnv("CACHE_USER_ID"), mustGetEnv("INDEXES_CACHE_ID")),
				TLSConfig: &tls.Config{
					MinVersion: tls.VersionTLS12,
				},
			},
			IPNIFindURL:            ipniFindURL,
			IPNIAnnounceAddrs:      []string{ipniPublisherAnnounceAddress},
			IPNIDirectAnnounceURLs: ipniPublisherDirectAnnounceURLs,
		},
		ProvidersCacheExpirationSeconds:   mustGetInt("PROVIDERS_CACHE_EXPIRATION_SECONDS"),
		NoProvidersCacheExpirationSeconds: mustGetInt("NO_PROVIDERS_CACHE_EXPIRATION_SECONDS"),
		ClaimsCacheExpirationSeconds:      mustGetInt("CLAIMS_CACHE_EXPIRATION_SECONDS"),
		IndexesCacheExpirationSeconds:     mustGetInt("INDEXES_CACHE_EXPIRATION_SECONDS"),
		SQSCachingQueueID:                 mustGetEnv("PROVIDER_CACHING_QUEUE_ID"),
		CachingBucket:                     mustGetEnv("PROVIDER_CACHING_BUCKET_NAME"),
		SQSPublishingQueueID:              mustGetEnv("IPNI_PUBLISHING_QUEUE_ID"),
		PublishingBucket:                  mustGetEnv("IPNI_PUBLISHING_BUCKET_NAME"),
		ChunkLinksTableName:               mustGetEnv("CHUNK_LINKS_TABLE_ID"),
		MetadataTableName:                 mustGetEnv("METADATA_TABLE_ID"),
		IPNIStoreBucket:                   mustGetEnv("IPNI_STORE_BUCKET_NAME"),
		IPNIStorePrefix:                   ipniStoreKeyPrefix,
		NotifierHeadBucket:                mustGetEnv("NOTIFIER_HEAD_BUCKET_NAME"),
		ClaimStoreBucket:                  mustGetEnv("CLAIM_STORE_BUCKET_NAME"),
		ClaimStorePrefix:                  os.Getenv("CLAIM_STORE_KEY_PREFIX"),
		LegacyClaimsTableName:             mustGetEnv("LEGACY_CLAIMS_TABLE_NAME"),
		LegacyClaimsTableRegion:           mustGetEnv("LEGACY_CLAIMS_TABLE_REGION"),
		LegacyClaimsBucket:                mustGetEnv("LEGACY_CLAIMS_BUCKET_NAME"),
		LegacyBlockIndexTableName:         mustGetEnv("LEGACY_BLOCK_INDEX_TABLE_NAME"),
		LegacyBlockIndexTableRegion:       mustGetEnv("LEGACY_BLOCK_INDEX_TABLE_REGION"),
		LegacyStoreTableName:              mustGetEnv("LEGACY_STORE_TABLE_NAME"),
		LegacyStoreTableRegion:            mustGetEnv("LEGACY_STORE_TABLE_REGION"),
		LegacyBlobRegistryTableName:       mustGetEnv("LEGACY_BLOB_REGISTRY_TABLE_NAME"),
		LegacyBlobRegistryTableRegion:     mustGetEnv("LEGACY_BLOB_REGISTRY_TABLE_REGION"),
		LegacyAllocationsTableName:        mustGetEnv("LEGACY_ALLOCATIONS_TABLE_NAME"),
		LegacyAllocationsTableRegion:      mustGetEnv("LEGACY_ALLOCATIONS_TABLE_REGION"),
		LegacyDataBucketURL:               mustGetEnv("LEGACY_DATA_BUCKET_URL"),
		LegacyDotStorageBucketPrefixes:    legacyDotStorageBucketPrefixes,
		BaseTraceSampleRatio:              mustGetFloat("BASE_TRACE_SAMPLE_RATIO"),
		SentryDSN:                         os.Getenv("SENTRY_DSN"),
		SentryEnvironment:                 os.Getenv("SENTRY_ENVIRONMENT"),
		HoneycombAPIKey:                   os.Getenv("HONEYCOMB_API_KEY"),
		IPNIFormatPeerID:                  os.Getenv("IPNI_FORMAT_PEER_ID"),
		IPNIFormatEndpoint:                os.Getenv("IPNI_FORMAT_ENDPOINT"),
		PrincipalMapping:                  principalMapping,
	}
}

// Construct constructs types.Service from AWS deps for Lamda functions
func Construct(cfg Config) (types.Service, error) {
	httpClient := construct.DefaultHTTPClient()
	providersClient := goredis.NewClusterClient(&cfg.ProvidersRedis)
	noProvidersClient := goredis.NewClusterClient(&cfg.NoProviderRedis)
	claimsClient := goredis.NewClusterClient(&cfg.ClaimsRedis)
	indexesClient := goredis.NewClusterClient(&cfg.IndexesRedis)

	// instrument HTTP and redis clients if telemetry is enabled
	if cfg.HoneycombAPIKey != "" {
		httpClient = telemetry.InstrumentHTTPClient(construct.DefaultHTTPClient())
		providersClient = telemetry.InstrumentRedisClient(providersClient)
		noProvidersClient = telemetry.InstrumentRedisClient(noProvidersClient)
		claimsClient = telemetry.InstrumentRedisClient(claimsClient)
		indexesClient = telemetry.InstrumentRedisClient(indexesClient)
	}

	cachingQueue := NewSQSCachingQueue(cfg.Config, cfg.SQSCachingQueueID, cfg.CachingBucket)
	ipniStore := NewS3Store(cfg.Config, cfg.IPNIStoreBucket, cfg.IPNIStorePrefix)
	claimBucketStore := contentclaims.NewStoreFromBucket(NewS3Store(cfg.Config, cfg.ClaimStoreBucket, cfg.ClaimStorePrefix))
	chunkLinksTable := NewDynamoProviderContextTable(cfg.Config, cfg.ChunkLinksTableName)
	metadataTable := NewDynamoProviderContextTable(cfg.Config, cfg.MetadataTableName)
	publisherStore := store.NewPublisherStore(ipniStore, chunkLinksTable, metadataTable, store.WithMetadataContext(metadata.MetadataContext))

	publishingQueue := awspublisherqueue.NewSQSPublishingQueue(cfg.Config, cfg.SQSPublishingQueueID, cfg.PublishingBucket)
	queuePublisher := publisherqueue.NewQueuePublisher(publishingQueue)

	legacyDataBucketURL, err := url.Parse(cfg.LegacyDataBucketURL)
	if err != nil {
		return nil, fmt.Errorf("parsing carpark url: %s", err)
	}
	// legacy claims mapper
	legacyClaimsCfg := cfg.Config.Copy()
	legacyClaimsCfg.Region = cfg.LegacyClaimsTableRegion
	legacyClaimsMapper := NewDynamoContentToClaimsMapper(dynamodb.NewFromConfig(legacyClaimsCfg), cfg.LegacyClaimsTableName)

	// bucket fallback mapper
	allocationsCfg := cfg.Config.Copy()
	allocationsCfg.Region = cfg.LegacyAllocationsTableRegion
	legacyAllocationsStore := NewDynamoAllocationsTable(dynamodb.NewFromConfig(allocationsCfg), cfg.LegacyAllocationsTableName)
	bucketFallbackMapper := NewBucketFallbackMapper(
		cfg.Signer,
		httpClient,
		legacyDataBucketURL,
		legacyAllocationsStore,
		func() []delegation.Option {
			return []delegation.Option{delegation.WithExpiration(int(time.Now().Add(time.Hour).Unix()))}
		},
	)

	// block index table mapper
	blockIndexCfg := cfg.Config.Copy()
	blockIndexCfg.Region = cfg.LegacyBlockIndexTableRegion
	legacyBlockIndexStore := NewDynamoProviderBlockIndexTable(dynamodb.NewFromConfig(blockIndexCfg), cfg.LegacyBlockIndexTableName)
	storeTableCfg := cfg.Config.Copy()
	storeTableCfg.Region = cfg.LegacyStoreTableRegion
	blobRegistryTableCfg := cfg.Config.Copy()
	blobRegistryTableCfg.Region = cfg.LegacyBlobRegistryTableRegion
	legacyMigratedShardChecker := NewDynamoMigratedShardChecker(
		cfg.LegacyStoreTableName,
		dynamodb.NewFromConfig(storeTableCfg),
		cfg.LegacyBlobRegistryTableName,
		dynamodb.NewFromConfig(blobRegistryTableCfg),
		legacyAllocationsStore,
	)
	// allow claims synthethized from the block index table to live longer after they are expired in the cache
	// so that the service doesn't return cached but expired delegations
	synthetizedClaimExp := time.Duration(cfg.ClaimsCacheExpirationSeconds)*time.Second + 1*time.Hour
	blockIndexTableMapper, err := NewBlockIndexTableMapper(cfg.Signer, legacyBlockIndexStore, legacyMigratedShardChecker, cfg.LegacyDataBucketURL, synthetizedClaimExp, cfg.LegacyDotStorageBucketPrefixes)
	if err != nil {
		return nil, fmt.Errorf("creating block index table mapper: %w", err)
	}

	legacyClaimsBucket := contentclaims.NewStoreFromBucket(NewS3Store(legacyClaimsCfg, cfg.LegacyClaimsBucket, ""))
	legacyClaimsURL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/{claim}/{claim}.car", cfg.LegacyClaimsBucket, cfg.Config.Region)

	var provIndexLog logging.EventLogger
	if cfg.SentryDSN != "" && cfg.SentryEnvironment != "" {
		err = sentry.Init(sentry.ClientOptions{
			Dsn:           cfg.SentryDSN,
			Environment:   cfg.SentryEnvironment,
			Release:       build.Version,
			Transport:     sentry.NewHTTPSyncTransport(),
			EnableTracing: false,
		})
		if err != nil {
			return nil, fmt.Errorf("initializing sentry: %w", err)
		}
		provIndexLog = telemetry.NewSentryLogger("providerindex")
	}

	service, err := construct.Construct(
		cfg.ServiceConfig,
		construct.SkipNotification(),
		construct.WithCachingQueue(cachingQueue),
		construct.WithPublisherStore(publisherStore),
		construct.WithAsyncPublisher(queuePublisher),
		construct.WithStartIPNIServer(false),
		construct.WithClaimsStore(claimBucketStore),
		construct.WithLegacyClaims([]legacy.ContentToClaimsMapper{legacyClaimsMapper, bucketFallbackMapper, blockIndexTableMapper}, legacyClaimsBucket, legacyClaimsURL),
		construct.WithHTTPClient(httpClient),
		construct.WithProvidersClient(redis.NewClusterClientAdapter(providersClient)),
		construct.WithNoProvidersClient(noProvidersClient),
		construct.WithClaimsClient(claimsClient),
		construct.WithIndexesClient(indexesClient),
		construct.WithProvidersCacheOptions(redis.ExpirationTime(time.Duration(cfg.ProvidersCacheExpirationSeconds)*time.Second)),
		construct.WithNoProvidersCacheOptions(redis.ExpirationTime(time.Duration(cfg.NoProvidersCacheExpirationSeconds)*time.Second)),
		construct.WithClaimsCacheOptions(redis.ExpirationTime(time.Duration(cfg.ClaimsCacheExpirationSeconds)*time.Second)),
		construct.WithIndexesCacheOptions(redis.ExpirationTime(time.Duration(cfg.IndexesCacheExpirationSeconds)*time.Second)),
		construct.WithProviderIndexLogger(provIndexLog),
	)
	if err != nil {
		return nil, err
	}

	return service, nil
}
