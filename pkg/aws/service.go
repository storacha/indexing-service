package aws

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/redis/go-redis/v9"
	"github.com/storacha/go-ucanto/principal"
	ed25519 "github.com/storacha/go-ucanto/principal/ed25519/signer"
	"github.com/storacha/indexing-service/pkg/construct"
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
	SQSCachingQueueURL  string
	CachingBucket       string
	ChunkLinksTableName string
	MetadataTableName   string
	IPNIStoreBucket     string
	IPNIStorePrefix     string
	NotifierHeadBucket  string
	NotifierTopicArn    string
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
	cryptoPrivKey, err := crypto.UnmarshalEd25519PrivateKey(id.Raw())
	if err != nil {
		panic(fmt.Errorf("unmarshaling private key: %w", err))
	}

	ipniStoreKeyPrefix := os.Getenv("IPNI_STORE_KEY_PREFIX")
	if len(ipniStoreKeyPrefix) == 0 {
		ipniStoreKeyPrefix = "/ipni/v1/ad/"
	}

	peer, err := peer.IDFromPrivateKey(cryptoPrivKey)
	if err != nil {
		panic(fmt.Errorf("parsing private key to peer: %w", err))
	}

	ipniPublisherAnnounceAddress := fmt.Sprintf("/dns4/%s/tcp/443/https/p2p/%s", mustGetEnv("IPNI_STORE_BUCKET_REGIONAL_DOMAIN"), peer.String())
	return Config{
		Config: awsConfig,
		Signer: id,
		ServiceConfig: construct.ServiceConfig{
			PrivateKey: cryptoPrivKey,
			ProvidersRedis: redis.Options{
				Addr:                       mustGetEnv("PROVIDERS_REDIS_URL"),
				CredentialsProviderContext: redisCredentialVerifier(awsConfig, mustGetEnv("REDIS_USER_ID"), mustGetEnv("PROVIDERS_REDIS_CACHE")),
			},
			ClaimsRedis: redis.Options{
				Addr:                       mustGetEnv("CLAIMS_REDIS_URL"),
				CredentialsProviderContext: redisCredentialVerifier(awsConfig, mustGetEnv("REDIS_USER_ID"), mustGetEnv("CLAIMS_REDIS_CACHE")),
			},
			IndexesRedis: redis.Options{
				Addr:                       mustGetEnv("INDEXES_REDIS_URL"),
				CredentialsProviderContext: redisCredentialVerifier(awsConfig, mustGetEnv("REDIS_USER_ID"), mustGetEnv("INDEXES_REDIS_CACHE")),
			},
			IndexerURL:             mustGetEnv("IPNI_ENDPOINT"),
			PublisherAnnounceAddrs: []string{ipniPublisherAnnounceAddress},
		},
		SQSCachingQueueURL:  mustGetEnv("PROVIDER_CACHING_QUEUE_URL"),
		CachingBucket:       mustGetEnv("PROVIDER_CACHING_BUCKET_NAME"),
		ChunkLinksTableName: mustGetEnv("CHUNK_LINKS_TABLE_NAME"),
		MetadataTableName:   mustGetEnv("METADATA_TABLE_NAME"),
		IPNIStoreBucket:     mustGetEnv("IPNI_STORE_BUCKET_NAME"),
		IPNIStorePrefix:     ipniStoreKeyPrefix,
		NotifierHeadBucket:  mustGetEnv("NOTIFIER_HEAD_BUCKET_NAME"),
		NotifierTopicArn:    mustGetEnv("NOTIFIER_SNS_TOPIC_ARN"),
	}
}

// Construct constructs types.Service from AWS deps for Lamda functions
func Construct(cfg Config) (types.Service, error) {
	cachingQueue := NewSQSCachingQueue(cfg.Config, cfg.SQSCachingQueueURL, cfg.CachingBucket)
	ipniStore := NewS3Store(cfg.Config, cfg.IPNIStoreBucket, cfg.IPNIStorePrefix)
	chunkLinksTable := NewDynamoProviderContextTable(cfg.Config, cfg.ChunkLinksTableName)
	metadataTable := NewDynamoProviderContextTable(cfg.Config, cfg.MetadataTableName)
	publisherStore := store.NewPublisherStore(ipniStore, chunkLinksTable, metadataTable)
	return construct.Construct(cfg.ServiceConfig,
		construct.SkipNotification(),
		construct.WithCachingQueue(cachingQueue),
		construct.WithPublisherStore(publisherStore),
		construct.WithStartIPNIServer(false))
}
