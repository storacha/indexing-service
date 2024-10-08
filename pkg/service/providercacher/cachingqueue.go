package providercacher

import (
	"context"

	"github.com/ipni/go-libipni/find/model"
	"github.com/storacha/indexing-service/pkg/blobindex"
)

type (
	ProviderCachingJob struct {
		provider model.ProviderResult
		index    blobindex.ShardedDagIndexView
	}

	JobQueue interface {
		Queue(ctx context.Context, j ProviderCachingJob) error
	}

	JobHandler struct {
		providerCacher ProviderCacher
	}

	CachingQueue struct {
		jobQueue JobQueue
	}
)

func NewJobHandler(providerCacher ProviderCacher) *JobHandler {
	return &JobHandler{
		providerCacher: providerCacher,
	}
}

func (j *JobHandler) Handle(ctx context.Context, job ProviderCachingJob) error {
	_, err := j.providerCacher.CacheProviderForIndexRecords(ctx, job.provider, job.index)
	return err
}

func NewCachingQueue(jobQueue JobQueue) *CachingQueue {
	return &CachingQueue{
		jobQueue: jobQueue,
	}
}

func (q *CachingQueue) QueueProviderCaching(ctx context.Context, provider model.ProviderResult, index blobindex.ShardedDagIndexView) error {
	return q.jobQueue.Queue(ctx, ProviderCachingJob{provider: provider, index: index})
}
