package providercacher

import (
	"context"

	"github.com/ipni/go-libipni/find/model"
	"github.com/storacha/indexing-service/pkg/blobindex"
)

type (
	CachingQueueQueuer interface {
		Queue(ctx context.Context, job ProviderCachingJob) error
	}

	CachingQueueReader interface {
		Read(ctx context.Context, maxJobs int) ([]ProviderCachingJob, error)
	}

	CachingQueueReleaser interface {
		Release(ctx context.Context, jobID string) error
	}

	CachingQueueDeleter interface {
		Delete(ctx context.Context, jobID string) error
	}

	CachingQueue interface {
		CachingQueueQueuer
		CachingQueueReader
		CachingQueueReleaser
		CachingQueueDeleter
	}

	ProviderCachingJob struct {
		ID       string
		Provider model.ProviderResult
		Index    blobindex.ShardedDagIndexView
	}

	JobHandler struct {
		providerCacher ProviderCacher
	}
)

func NewJobHandler(providerCacher ProviderCacher) *JobHandler {
	return &JobHandler{
		providerCacher: providerCacher,
	}
}

func (j *JobHandler) Handle(ctx context.Context, job ProviderCachingJob) error {
	return j.providerCacher.CacheProviderForIndexRecords(ctx, job.Provider, job.Index)
}
