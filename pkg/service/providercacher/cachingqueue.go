package providercacher

import (
	"context"

	"github.com/ipni/go-libipni/find/model"
	"github.com/storacha/indexing-service/pkg/blobindex"
)

type (
	ProviderCachingJob struct {
		Provider model.ProviderResult
		Index    blobindex.ShardedDagIndexView
	}

	JobQueue interface {
		Queue(ctx context.Context, j ProviderCachingJob) error
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
