package providercacher

import (
	"context"
)

type JobHandler struct {
	providerCacher ProviderCachingQueue
}

func NewJobHandler(providerCacher ProviderCachingQueue) *JobHandler {
	return &JobHandler{
		providerCacher: providerCacher,
	}
}

func (j *JobHandler) Handle(ctx context.Context, job CacheProviderMessage) error {
	return j.providerCacher.Queue(ctx, job)
}
