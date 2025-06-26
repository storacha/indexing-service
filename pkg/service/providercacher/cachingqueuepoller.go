package providercacher

import (
	"context"
	"errors"
	"sync"
	"time"

	logging "github.com/ipfs/go-log/v2"
)

const (
	defaultPollInterval = 5 * time.Second
	defaultJobBatchSize = 10
)

var log = logging.Logger("service/providercacher")

// CachingQueuePoller polls a queue for provider caching jobs and processes them
// using the provided ProviderCacher and SQSCachingDecoder.
type CachingQueuePoller struct {
	queue        CachingQueue
	cacher       ProviderCacher
	interval     time.Duration
	jobBatchSize int
	// Channel to signal the poller to stop
	done chan struct{}
	// Channel to confirm the poller has stopped
	stopped chan struct{}
}

// Option configures the CachingQueuePoller
type Option func(*CachingQueuePoller)

// WithPollInterval sets the polling interval for the queue
func WithPollInterval(interval time.Duration) Option {
	return func(p *CachingQueuePoller) {
		p.interval = interval
	}
}

// WithJobBatchSize sets the maximum number of jobs to process in a batch
func WithJobBatchSize(size int) Option {
	return func(p *CachingQueuePoller) {
		p.jobBatchSize = size
	}
}

// NewCachingQueuePoller creates a new CachingQueuePoller instance.
func NewCachingQueuePoller(queue CachingQueue, cacher ProviderCacher, opts ...Option) *CachingQueuePoller {
	poller := &CachingQueuePoller{
		queue:        queue,
		cacher:       cacher,
		interval:     defaultPollInterval,
		jobBatchSize: defaultJobBatchSize,
		done:         make(chan struct{}),
		stopped:      make(chan struct{}),
	}

	// Apply options
	for _, opt := range opts {
		opt(poller)
	}

	return poller
}

// Start begins polling the queue for caching jobs.
func (p *CachingQueuePoller) Start() {
	log.Info("Starting caching queue poller")

	// Start the polling loop in a goroutine
	go func() {
		ticker := time.NewTicker(p.interval)

		for {
			select {
			case <-p.done:
				log.Info("Stopping caching queue poller")
				close(p.stopped)
				return
			case <-ticker.C:
				p.pollJobs()
			}
		}
	}()
}

// Stop gracefully shuts down the poller.
func (p *CachingQueuePoller) Stop() {
	if p.done == nil {
		return
	}

	// Signal the poller to stop
	close(p.done)
	// Wait for the poller to stop
	<-p.stopped
}

// pollJobs reads and processes a batch of caching jobs from the queue
func (p *CachingQueuePoller) pollJobs() {
	ctx := context.Background()

	// Read a batch of jobs
	jobs, err := p.queue.ReadJobs(ctx, p.jobBatchSize)
	if err != nil {
		return
	}

	if len(jobs) == 0 {
		// No new jobs this time, will retry on next interval
		return
	}

	// process jobs in parallel
	errs := make(chan error, len(jobs))
	var wg sync.WaitGroup
	for _, job := range jobs {
		wg.Add(1)
		go func(job ProviderCachingJob) {
			defer wg.Done()
			err := p.cacher.CacheProviderForIndexRecords(ctx, job.Provider, job.Index)
			if err != nil {
				errs <- err
				return
			}

			// Delete the job if processing was successful
			if err := p.queue.DeleteJob(ctx, job.ID); err != nil {
				errs <- err
				return
			}
		}(job)
	}
	wg.Wait()

	// collect errors
	close(errs)
	for e := range errs {
		err = errors.Join(err, e)
	}

	if err != nil {
		log.Errorf("Failed to process messages: %v", err)
	}
}
