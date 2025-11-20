package providercacher

import (
	"github.com/storacha/go-libstoracha/queuepoller"
)

// CachingQueuePoller polls a queue for provider caching jobs and processes them
// using the provided ProviderCacher and SQSCachingDecoder.
type CachingQueuePoller = queuepoller.QueuePoller[ProviderCachingJob]

// NewCachingQueuePoller creates a new CachingQueuePoller instance.
func NewCachingQueuePoller(queue CachingQueue, cacher ProviderCacher, opts ...queuepoller.Option) (*CachingQueuePoller, error) {
	return queuepoller.NewQueuePoller(queue, queuepoller.JobHandler(NewJobHandler(cacher).Handle), opts...)
}
