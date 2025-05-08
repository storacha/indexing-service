package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/google/uuid"
	"github.com/ipni/go-libipni/find/model"
	"github.com/multiformats/go-multihash"
	"github.com/storacha/indexing-service/pkg/service/providercacher"
)

// MaxDigests is the maximum number of digests to include in an SQS message.
//
// sha2-256 multihash will encode as base64 string of 48 bytes:
// e.g. EiADkFjG8sDLSSxTOwpNFO93zA94q8zO1Sh9hKGiARz7gQ==
//
// Add extra 3 bytes for quotes and comma separator:
// e.g. "EiADkFjG8sDLSSxTOwpNFO93zA94q8zO1Sh9hKGiARz7gQ==",
//
// Max message size is 262,144 bytes (256 KiB) which would allow this many
// hashes in total:
// (48 + 3) / 262,144 = ~5140
//
// However, the message also has the provider result, so lets leave ample space
// for this and allow up to 1,000 hashes per message.
//
// SQS message batch allows up to 10 messages to be sent so we can enqueue a
// 10k NFT / ~10GB of data (assuming 1MB chunk size) in a single request.
var MaxDigests = 1_000

// MaxBatchEntries is the max number of messages allowed in a SQS message batch.
// https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/quotas-messages.html
var MaxBatchEntries = 10

// CachingQueueMessage is the struct that is serialized onto an SQS message queue in JSON
type CachingQueueMessage struct {
	Provider model.ProviderResult  `json:"Provider,omitempty"`
	Digests  []multihash.Multihash `json:"Digests,omitempty"`
}

// SQSCachingQueue implements the providercacher.CachingQueue interface using SQS
type SQSCachingQueue struct {
	queueURL  string
	sqsClient *sqs.Client
}

// NewSQSCachingQueue returns a new SQSCachingQueue for the given aws config
func NewSQSCachingQueue(cfg aws.Config, queueURL string) *SQSCachingQueue {
	return &SQSCachingQueue{
		queueURL:  queueURL,
		sqsClient: sqs.NewFromConfig(cfg),
	}
}

// Queue implements [providercacher.ProviderCachingQueue].
func (s *SQSCachingQueue) Queue(ctx context.Context, msg providercacher.CacheProviderMessage) error {
	var batch []CachingQueueMessage
	qmsg := CachingQueueMessage{Provider: msg.Provider}
	for digest := range msg.Digests {
		qmsg.Digests = append(qmsg.Digests, digest)

		if len(qmsg.Digests) >= MaxDigests {
			batch = append(batch, qmsg)

			if len(batch) >= MaxBatchEntries {
				err := s.sendMessage(ctx, batch)
				if err != nil {
					return fmt.Errorf("sending batch: %w", err)
				}
				batch = []CachingQueueMessage{}
			}

			qmsg = CachingQueueMessage{Provider: msg.Provider}
		}
	}

	if len(qmsg.Digests) > 0 {
		batch = append(batch, qmsg)
	}

	if len(batch) > 0 {
		err := s.sendMessage(ctx, batch)
		if err != nil {
			return fmt.Errorf("sending final batch: %w", err)
		}
	}

	return nil
}

func (s *SQSCachingQueue) sendMessage(ctx context.Context, msgs []CachingQueueMessage) error {
	var entries []types.SendMessageBatchRequestEntry
	for _, m := range msgs {
		body, err := json.Marshal(m)
		if err != nil {
			return fmt.Errorf("marshaling message: %w", err)
		}
		entries = append(entries, types.SendMessageBatchRequestEntry{
			Id:          aws.String(uuid.New().String()),
			MessageBody: aws.String(string(body)),
		})
	}

	_, err := s.sqsClient.SendMessageBatch(ctx, &sqs.SendMessageBatchInput{
		QueueUrl: aws.String(s.queueURL),
		Entries:  entries,
	})
	if err != nil {
		return fmt.Errorf("sending batch: %w", err)
	}
	return nil
}

var _ providercacher.ProviderCachingQueue = (*SQSCachingQueue)(nil)

// SQSCachingDecoder provides interfaces for working with caching jobs received over SQS
type SQSCachingDecoder struct{}

// NewSQSCachingDecoder returns a new decoder for the given AWS config
func NewSQSCachingDecoder() *SQSCachingDecoder {
	return &SQSCachingDecoder{}
}

// DecodeMessage decodes a provider caching job from the SQS message body, reading the stored index from S3
func (s *SQSCachingDecoder) DecodeMessage(ctx context.Context, messageBody string) (providercacher.CacheProviderMessage, error) {
	var msg CachingQueueMessage
	err := json.Unmarshal([]byte(messageBody), &msg)
	if err != nil {
		return providercacher.CacheProviderMessage{}, fmt.Errorf("deserializing message: %w", err)
	}
	return providercacher.CacheProviderMessage{
		Provider: msg.Provider,
		Digests:  slices.Values(msg.Digests),
	}, nil
}
