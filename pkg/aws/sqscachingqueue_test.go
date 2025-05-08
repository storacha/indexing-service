package aws

import (
	"context"
	"fmt"
	"iter"
	"net/url"
	"os"
	"runtime"
	"slices"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/docker/go-connections/nat"
	"github.com/google/uuid"
	"github.com/multiformats/go-multihash"
	"github.com/storacha/indexing-service/pkg/bytemap"
	"github.com/storacha/indexing-service/pkg/internal/testutil"
	"github.com/storacha/indexing-service/pkg/service/providercacher"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
)

func TestSqsCachingQueue(t *testing.T) {
	if os.Getenv("CI") != "" && runtime.GOOS != "linux" {
		t.SkipNow()
	}

	ctx := context.Background()
	endpoint := createSQS(t)
	sqsClient := newSqsClient(t, endpoint)

	t.Run("batch messages", func(t *testing.T) {
		// In ElasticMQ the limit is 64KiB (65,536 bytes)
		maxDigests := MaxDigests
		MaxDigests = 300
		t.Cleanup(func() { MaxDigests = maxDigests })

		tableName := "provider-caching-" + uuid.NewString()
		queueURL := createQueue(t, sqsClient, tableName)

		cachingQueue := &SQSCachingQueue{
			queueURL:  queueURL.String(),
			sqsClient: sqsClient,
		}

		provider := testutil.RandomProviderResult()
		digests := testutil.RandomMultihashes(10_000)

		err := cachingQueue.Queue(ctx, providercacher.CacheProviderMessage{
			Provider: provider,
			Digests:  slices.Values(digests),
		})
		require.NoError(t, err)

		expectedDigests := bytemap.NewByteMap[multihash.Multihash, struct{}](len(digests))
		for _, d := range digests {
			expectedDigests.Set(d, struct{}{})
		}

		decoder := SQSCachingDecoder{}
		rawMessages := drainQueue(t, sqsClient, queueURL)
		for m := range decodeMessages(t, decoder, rawMessages) {
			require.Equal(t, provider, m.Provider)
			for d := range m.Digests {
				require.True(t, expectedDigests.Delete(d))
			}
		}
		require.Equal(t, expectedDigests.Size(), 0)
	})
}

func drainQueue(t *testing.T, sqsClient *sqs.Client, queueURL url.URL) iter.Seq[types.Message] {
	return func(yield func(m types.Message) bool) {
		for {
			res, err := sqsClient.ReceiveMessage(context.Background(), &sqs.ReceiveMessageInput{
				QueueUrl:            aws.String(queueURL.String()),
				MaxNumberOfMessages: 10,
			})
			require.NoError(t, err)
			if len(res.Messages) == 0 {
				return
			}
			for _, m := range res.Messages {
				if !yield(m) {
					return
				}
			}
		}
	}
}

func decodeMessages(t *testing.T, decoder SQSCachingDecoder, rawMessages iter.Seq[types.Message]) iter.Seq[providercacher.CacheProviderMessage] {
	return func(yield func(m providercacher.CacheProviderMessage) bool) {
		for m := range rawMessages {
			msg, err := decoder.DecodeMessage(context.Background(), *m.Body)
			require.NoError(t, err)
			if !yield(msg) {
				return
			}
		}
	}
}

func createSQS(t *testing.T) url.URL {
	ctx := context.Background()
	container, err := testcontainers.Run(
		ctx,
		"softwaremill/elasticmq-native:latest",
		testcontainers.WithExposedPorts("9324"),
	)
	testcontainers.CleanupContainer(t, container)
	require.NoError(t, err)

	host, err := container.Host(ctx)
	require.NoError(t, err)

	port, err := container.MappedPort(ctx, nat.Port("9324/tcp"))
	require.NoError(t, err)

	endpoint, err := url.Parse(fmt.Sprintf("http://%s:%d", host, port.Int()))
	require.NoError(t, err)

	return *endpoint
}

func newSqsClient(t *testing.T, endpoint url.URL) *sqs.Client {
	cfg, err := config.LoadDefaultConfig(
		context.Background(),
		config.WithCredentialsProvider(credentials.StaticCredentialsProvider{
			Value: aws.Credentials{
				AccessKeyID:     "DUMMYIDEXAMPLE",
				SecretAccessKey: "DUMMYEXAMPLEKEY",
			},
		}),
		func(o *config.LoadOptions) error {
			o.Region = "elasticmq"
			return nil
		},
	)
	require.NoError(t, err)

	return sqs.NewFromConfig(cfg, func(o *sqs.Options) {
		base := endpoint.String()
		o.BaseEndpoint = &base
	})
}

func createQueue(t *testing.T, sqsClient *sqs.Client, name string) url.URL {
	res, err := sqsClient.CreateQueue(context.Background(), &sqs.CreateQueueInput{
		QueueName: aws.String(name),
	})
	require.NoError(t, err)

	queueURL, err := url.Parse(*res.QueueUrl)
	require.NoError(t, err)
	return *queueURL
}
