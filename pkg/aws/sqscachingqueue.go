package aws

import (
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/ipni/go-libipni/find/model"
	"github.com/storacha/go-libstoracha/awsutils"
	"github.com/storacha/go-libstoracha/blobindex"
	"github.com/storacha/indexing-service/pkg/service/providercacher"
)

var _ providercacher.CachingQueue = (*SQSCachingQueue)(nil)

// SQSCachingQueue implements the providercacher.CachingQueue interface using SQS
type SQSCachingQueue = awsutils.SQSExtendedQueue[providercacher.ProviderCachingJob, model.ProviderResult]

type jobMarshaller struct{}

func (jm jobMarshaller) Marshall(job providercacher.ProviderCachingJob) (awsutils.SerializedJob[model.ProviderResult], error) {
	reader, err := job.Index.Archive()
	if err != nil {
		return awsutils.SerializedJob[model.ProviderResult]{}, fmt.Errorf("serializing index to CAR: %w", err)
	}
	return awsutils.SerializedJob[model.ProviderResult]{
		ID:       job.ID,
		Message:  job.Provider,
		Extended: reader,
	}, nil
}

func (jm jobMarshaller) Unmarshall(sj awsutils.SerializedJob[model.ProviderResult]) (providercacher.ProviderCachingJob, error) {
	index, err := blobindex.Extract(sj.Extended)
	if err != nil {
		return providercacher.ProviderCachingJob{}, fmt.Errorf("deserializing index from CAR: %w", err)
	}
	return providercacher.ProviderCachingJob{
		ID:       sj.ID,
		Provider: sj.Message,
		Index:    index,
	}, nil
}

func (jm jobMarshaller) Empty() providercacher.ProviderCachingJob {
	return providercacher.ProviderCachingJob{}
}

// NewSQSCachingQueue returns a new SQSCachingQueue for the given aws config
func NewSQSCachingQueue(cfg aws.Config, queueID string, bucket string) *SQSCachingQueue {
	return awsutils.NewSQSExtendedQueue(cfg, queueID, bucket, jobMarshaller{})
}

// SQSCachingDecoder is an alias for the SQSDecoder specialized for providercacher.ProviderCachingJob
type SQSCachingDecoder = awsutils.SQSDecoder[providercacher.ProviderCachingJob, model.ProviderResult]

// NewSQSCachingDecoder returns a new SQSDecoder for the providercacher.ProviderCachingJob type
func NewSQSCachingDecoder(cfg aws.Config, bucket string) *SQSCachingDecoder {
	return awsutils.NewSQSDecoder(cfg, bucket, jobMarshaller{})
}
