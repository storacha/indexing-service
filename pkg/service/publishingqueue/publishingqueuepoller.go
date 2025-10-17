package publishingqueue

import (
	"github.com/storacha/go-libstoracha/ipnipublisher/publisher"
	"github.com/storacha/go-libstoracha/ipnipublisher/queue"
	"github.com/storacha/indexing-service/pkg/internal/queuepoller"
)

// PublisherQueue is a queue for publishing jobs
type PublisherQueue = queuepoller.Queue[queue.PublishingJob]

// PublishingQueuePoller polls a queue for provider publishing jobs and processes them
// using the provided Publisher
type PublishingQueuePoller = queuepoller.QueuePoller[queue.PublishingJob]

type publishingJobIdentifier struct{}

func (p publishingJobIdentifier) ID(job queue.PublishingJob) string {
	return job.ID
}

// NewPublishingQueuePoller creates a new PublishingQueuePoller instance.
func NewPublishingQueuePoller(publisherQueue PublisherQueue, publisher publisher.Publisher, opts ...queuepoller.Option) (*PublishingQueuePoller, error) {
	return queuepoller.NewQueuePoller(publisherQueue, queue.NewJobHandler(publisher), publishingJobIdentifier{}, opts...)
}
