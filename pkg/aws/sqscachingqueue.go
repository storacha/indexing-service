package aws

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/google/uuid"
	"github.com/ipni/go-libipni/find/model"
	"github.com/storacha/indexing-service/pkg/blobindex"
	"github.com/storacha/indexing-service/pkg/service/blobindexlookup"
	"github.com/storacha/indexing-service/pkg/service/providercacher"
)

// CachingQueueMessage is the struct that is serialized onto an SQS message queue in JSON
type CachingQueueMessage struct {
	JobID    uuid.UUID            `json:"JobID,omitempty"`
	Provider model.ProviderResult `json:"Provider,omitempty"`
}

// SQSCachingQueue implements the providercacher.CachingQueue interface using SQS
type SQSCachingQueue struct {
	queueURL  string
	bucket    string
	s3Client  *s3.Client
	sqsClient *sqs.Client
}

// NewSQSCachingQueue returns a new SQSCachingQueue for the given aws config
func NewSQSCachingQueue(cfg aws.Config, queurURL string, bucket string) *SQSCachingQueue {
	return &SQSCachingQueue{
		queueURL:  queurURL,
		bucket:    bucket,
		s3Client:  s3.NewFromConfig(cfg),
		sqsClient: sqs.NewFromConfig(cfg),
	}
}

// Queue implements blobindexlookup.CachingQueue.
func (s *SQSCachingQueue) Queue(ctx context.Context, job providercacher.ProviderCachingJob) error {
	uuid := uuid.New()
	r, err := job.Index.Archive()
	if err != nil {
		return fmt.Errorf("serializing index to CAR: %w", err)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("reading index from CAR: %w", err)
	}
	_, err = s.s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(uuid.String()),
		Body:          bytes.NewReader(data),
		ContentLength: aws.Int64(int64(len(data))),
	})
	if err != nil {
		return fmt.Errorf("saving index CAR to S3: %w", err)
	}
	err = s.sendMessage(ctx, CachingQueueMessage{
		JobID:    uuid,
		Provider: job.Provider,
	})
	if err != nil {
		// error sending message so cleanup queue
		_, s3deleteErr := s.s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(s.bucket),
			Key:    aws.String(uuid.String()),
		})
		if s3deleteErr != nil {
			err = errors.Join(err, fmt.Errorf("cleaning up index CAR on S3: %w", s3deleteErr))
		}
	}
	return err
}

func (s *SQSCachingQueue) sendMessage(ctx context.Context, msg CachingQueueMessage) error {

	messageJSON, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("serializing message json: %w", err)
	}
	_, err = s.sqsClient.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:       aws.String(s.queueURL),
		MessageBody:    aws.String(string(messageJSON)),
		MessageGroupId: aws.String("default"),
	})
	if err != nil {
		return fmt.Errorf("enqueueing message: %w", err)
	}
	return nil
}

var _ blobindexlookup.CachingQueue = (*SQSCachingQueue)(nil)

// SQSCachingDecoder provides interfaces for working with caching jobs received over SQS
type SQSCachingDecoder struct {
	bucket   string
	s3Client *s3.Client
}

// NewSQSCachingDecoder returns a new decoder for the given AWS config
func NewSQSCachingDecoder(cfg aws.Config, bucket string) *SQSCachingDecoder {
	return &SQSCachingDecoder{
		bucket:   bucket,
		s3Client: s3.NewFromConfig(cfg),
	}
}

// DecodeMessage decodes a provider caching job from the SQS message body, reading the stored index from S3
func (s *SQSCachingDecoder) DecodeMessage(ctx context.Context, messageBody string) (providercacher.ProviderCachingJob, error) {
	var msg CachingQueueMessage
	err := json.Unmarshal([]byte(messageBody), &msg)
	if err != nil {
		return providercacher.ProviderCachingJob{}, fmt.Errorf("deserializing message: %w", err)
	}
	received, err := s.s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(msg.JobID.String()),
	})
	if err != nil {
		return providercacher.ProviderCachingJob{}, fmt.Errorf("reading stored index CAR: %w", err)
	}
	defer received.Body.Close()
	index, err := blobindex.Extract(received.Body)
	if err != nil {
		return providercacher.ProviderCachingJob{}, fmt.Errorf("deserializing index: %w", err)
	}
	return providercacher.ProviderCachingJob{Provider: msg.Provider, Index: index}, nil
}
