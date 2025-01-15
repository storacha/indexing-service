package aws

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/redis/go-redis/v9"
	"github.com/storacha/go-metadata"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/go-ucanto/did"
	"github.com/storacha/go-ucanto/principal"
	ed25519 "github.com/storacha/go-ucanto/principal/ed25519/signer"
	"github.com/storacha/go-ucanto/principal/signer"
	"github.com/storacha/indexing-service/pkg/construct"
	"github.com/storacha/indexing-service/pkg/service/contentclaims"
	"github.com/storacha/indexing-service/pkg/service/legacy"
	"github.com/storacha/indexing-service/pkg/types"
	"github.com/storacha/ipni-publisher/pkg/store"
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

// Config describes all the values required to setup AWS from the environment
type Config struct {
	construct.ServiceConfig
	aws.Config
	SQSCachingQueueURL          string
	CachingBucket               string
	ChunkLinksTableName         string
	MetadataTableName           string
	IPNIStoreBucket             string
	IPNIStorePrefix             string
	NotifierHeadBucket          string
	NotifierTopicArn            string
	ClaimStoreBucket            string
	ClaimStorePrefix            string
	LegacyClaimsTableName       string
	LegacyClaimsTableRegion     string
	LegacyClaimsBucket          string
	LegacyBlockIndexTableName   string
	LegacyBlockIndexTableRegion string
	LegacyDataBucketURL         string
	HoneycombAPIKey             string
	principal.Signer
}

// FromEnv constructs the AWS Configuration from the environment
func FromEnv(ctx context.Context) Config {
	awsConfig, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		panic(fmt.Errorf("loading aws default config: %w", err))
	}
	ssmClient := ssm.NewFromConfig(awsConfig)
	response, err := ssmClient.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           aws.String(mustGetEnv("PRIVATE_KEY")),
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		panic(fmt.Errorf("retrieving private key: %w", err))
	}
	if response.Parameter == nil || response.Parameter.Value == nil {
		panic(ErrNoPrivateKey)
	}
	id, err := ed25519.Parse(*response.Parameter.Value)
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

	var honeycombAPIKey string
	response, err = ssmClient.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           aws.String(mustGetEnv("HONEYCOMB_API_KEY_PARAM")),
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		panic(fmt.Errorf("retrieving honeycomb API key: %w", err))
	}
	if response.Parameter != nil && response.Parameter.Value != nil {
		honeycombAPIKey = *response.Parameter.Value
	}

	return Config{
		Config: awsConfig,
		Signer: id,
		ServiceConfig: construct.ServiceConfig{
			PrivateKey: cryptoPrivKey,
			PublicURL:  strings.Split(mustGetEnv("PUBLIC_URL"), ","),
			ProvidersRedis: redis.Options{
				Addr:                       mustGetEnv("PROVIDERS_REDIS_URL") + ":6379",
				CredentialsProviderContext: redisCredentialVerifier(awsConfig, mustGetEnv("REDIS_USER_ID"), mustGetEnv("PROVIDERS_REDIS_CACHE")),
				TLSConfig: &tls.Config{
					MinVersion: tls.VersionTLS12,
				},
			},
			ClaimsRedis: redis.Options{
				Addr:                       mustGetEnv("CLAIMS_REDIS_URL") + ":6379",
				CredentialsProviderContext: redisCredentialVerifier(awsConfig, mustGetEnv("REDIS_USER_ID"), mustGetEnv("CLAIMS_REDIS_CACHE")),
				TLSConfig: &tls.Config{
					MinVersion: tls.VersionTLS12,
				},
			},
			IndexesRedis: redis.Options{
				Addr:                       mustGetEnv("INDEXES_REDIS_URL") + ":6379",
				CredentialsProviderContext: redisCredentialVerifier(awsConfig, mustGetEnv("REDIS_USER_ID"), mustGetEnv("INDEXES_REDIS_CACHE")),
				TLSConfig: &tls.Config{
					MinVersion: tls.VersionTLS12,
				},
			},
			IndexerURL:             mustGetEnv("IPNI_ENDPOINT"),
			PublisherAnnounceAddrs: []string{ipniPublisherAnnounceAddress},
		},
		SQSCachingQueueURL:          mustGetEnv("PROVIDER_CACHING_QUEUE_URL"),
		CachingBucket:               mustGetEnv("PROVIDER_CACHING_BUCKET_NAME"),
		ChunkLinksTableName:         mustGetEnv("CHUNK_LINKS_TABLE_NAME"),
		MetadataTableName:           mustGetEnv("METADATA_TABLE_NAME"),
		IPNIStoreBucket:             mustGetEnv("IPNI_STORE_BUCKET_NAME"),
		IPNIStorePrefix:             ipniStoreKeyPrefix,
		NotifierHeadBucket:          mustGetEnv("NOTIFIER_HEAD_BUCKET_NAME"),
		NotifierTopicArn:            mustGetEnv("NOTIFIER_SNS_TOPIC_ARN"),
		ClaimStoreBucket:            mustGetEnv("CLAIM_STORE_BUCKET_NAME"),
		ClaimStorePrefix:            os.Getenv("CLAIM_STORE_KEY_REFIX"),
		LegacyClaimsTableName:       mustGetEnv("LEGACY_CLAIMS_TABLE_NAME"),
		LegacyClaimsTableRegion:     mustGetEnv("LEGACY_CLAIMS_TABLE_REGION"),
		LegacyClaimsBucket:          mustGetEnv("LEGACY_CLAIMS_BUCKET_NAME"),
		LegacyBlockIndexTableName:   mustGetEnv("LEGACY_BLOCK_INDEX_TABLE_NAME"),
		LegacyBlockIndexTableRegion: mustGetEnv("LEGACY_BLOCK_INDEX_TABLE_REGION"),
		LegacyDataBucketURL:         mustGetEnv("LEGACY_DATA_BUCKET_URL"),
		HoneycombAPIKey:             honeycombAPIKey,
	}
}

// Construct constructs types.Service from AWS deps for Lamda functions
func Construct(cfg Config) (types.Service, error) {
	cachingQueue := NewSQSCachingQueue(cfg.Config, cfg.SQSCachingQueueURL, cfg.CachingBucket)
	ipniStore := NewS3Store(cfg.Config, cfg.IPNIStoreBucket, cfg.IPNIStorePrefix)
	claimBucketStore := contentclaims.NewStoreFromBucket(NewS3Store(cfg.Config, cfg.ClaimStoreBucket, cfg.ClaimStorePrefix))
	chunkLinksTable := NewDynamoProviderContextTable(cfg.Config, cfg.ChunkLinksTableName)
	metadataTable := NewDynamoProviderContextTable(cfg.Config, cfg.MetadataTableName)
	publisherStore := store.NewPublisherStore(ipniStore, chunkLinksTable, metadataTable, store.WithMetadataContext(metadata.MetadataContext))
	legacyDataBucketURL, err := url.Parse(cfg.LegacyDataBucketURL)
	if err != nil {
		return nil, fmt.Errorf("parsing carpark url: %s", err)
	}

	legacyClaimsCfg := cfg.Config.Copy()
	legacyClaimsCfg.Region = cfg.LegacyClaimsTableRegion
	legacyClaimsMapper := NewBucketFallbackMapper(
		cfg.Signer,
		legacyDataBucketURL,
		NewDynamoContentToClaimsMapper(dynamodb.NewFromConfig(legacyClaimsCfg), cfg.LegacyClaimsTableName),
		func() []delegation.Option {
			return []delegation.Option{delegation.WithExpiration(int(time.Now().Add(time.Hour).Unix()))}
		},
	)
	legacyClaimsBucket := contentclaims.NewStoreFromBucket(NewS3Store(legacyClaimsCfg, cfg.LegacyClaimsBucket, ""))
	legacyClaimsURL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/{claim}/{claim}.car", cfg.LegacyClaimsBucket, cfg.Config.Region)

	service, err := construct.Construct(
		cfg.ServiceConfig,
		construct.SkipNotification(),
		construct.WithCachingQueue(cachingQueue),
		construct.WithPublisherStore(publisherStore),
		construct.WithStartIPNIServer(false),
		construct.WithClaimsStore(claimBucketStore),
		construct.WithLegacyClaims(legacyClaimsMapper, legacyClaimsBucket, legacyClaimsURL),
	)
	if err != nil {
		return nil, err
	}
	blockIndexCfg := cfg.Config.Copy()
	blockIndexCfg.Region = cfg.LegacyBlockIndexTableRegion
	legacyBlockIndexStore := NewDynamoProviderBlockIndexTable(dynamodb.NewFromConfig(blockIndexCfg), cfg.LegacyBlockIndexTableName)
	return legacy.NewService(cfg.Signer, service, legacyBlockIndexStore, cfg.LegacyDataBucketURL)
}
