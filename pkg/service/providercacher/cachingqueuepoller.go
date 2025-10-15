package providercacher

import (
	"github.com/storacha/indexing-service/pkg/internal/queuepoller"
)

// CachingQueuePoller polls a queue for provider caching jobs and processes them
// using the provided ProviderCacher and SQSCachingDecoder.
type CachingQueuePoller = queuepoller.QueuePoller[ProviderCachingJob]

type providerJobIdentifier struct{}

func (p providerJobIdentifier) ID(job ProviderCachingJob) string {
	return job.ID
}

// NewCachingQueuePoller creates a new CachingQueuePoller instance.
func NewCachingQueuePoller(queue CachingQueue, cacher ProviderCacher, opts ...queuepoller.Option) (*CachingQueuePoller, error) {
	return queuepoller.NewQueuePoller(queue, NewJobHandler(cacher), providerJobIdentifier{}, opts...)
}
