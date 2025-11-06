package providercacher

import (
	"context"

	"github.com/ipni/go-libipni/find/model"
	"github.com/storacha/go-libstoracha/blobindex"
	"github.com/storacha/go-libstoracha/queuepoller"
)

type (
	CachingQueueQueuer = queuepoller.QueueQueuer[ProviderCachingJob]
	CachingQueue       = queuepoller.Queue[ProviderCachingJob]

	ProviderCachingJob struct {
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
