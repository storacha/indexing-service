package providercacher

import (
	"context"
	"fmt"
	"sync"
	"time"

	logging "github.com/ipfs/go-log/v2"
)

const (
	defaultPollInterval = 5 * time.Second
	defaultJobBatchSize = 10

	maxJobBatchSize    = 10
	maxParallelBatches = 100
)

var log = logging.Logger("service/providercacher")

// CachingQueuePoller polls a queue for provider caching jobs and processes them
// using the provided ProviderCacher and SQSCachingDecoder.
type CachingQueuePoller struct {
	queue        CachingQueue
	cacher       ProviderCacher
	interval     time.Duration
	jobBatchSize int
	ctx          context.Context
	cancel       context.CancelFunc
	stopped      chan struct{}
	startOnce    sync.Once
	stopOnce     sync.Once
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
func NewCachingQueuePoller(queue CachingQueue, cacher ProviderCacher, opts ...Option) (*CachingQueuePoller, error) {
	poller := &CachingQueuePoller{
		queue:        queue,
		cacher:       cacher,
		interval:     defaultPollInterval,
		jobBatchSize: defaultJobBatchSize,
		stopped:      make(chan struct{}),
	}

	// Apply options
	for _, opt := range opts {
		opt(poller)
	}

	if poller.interval <= 0 {
		return nil, fmt.Errorf("poll interval %v must be positive", poller.interval)
	}

	if poller.jobBatchSize > maxJobBatchSize {
		return nil, fmt.Errorf("job batch size %d exceeds maximum allowed %d", poller.jobBatchSize, maxJobBatchSize)
	}

	return poller, nil
}

// Start begins polling the queue for caching jobs.
func (p *CachingQueuePoller) Start() {
	p.startOnce.Do(func() {
		// Create root context
		p.ctx, p.cancel = context.WithCancel(context.Background())
		log.Info("Starting caching queue poller")

		go func() {
			timer := time.NewTimer(p.interval)

			for {
				select {
				case <-timer.C:
					p.processJobs(p.ctx)
					timer.Reset(p.interval)

				case <-p.ctx.Done():
					log.Info("Stopping polling loop")
					close(p.stopped)
					return
				}
			}
		}()
	})
}

// Stop stops the polling loop and waits for it to finish.
func (p *CachingQueuePoller) Stop() {
	p.stopOnce.Do(func() {
		// Cancel the root context, which will cancel all child contexts
		if p.cancel != nil {
			p.cancel()
		}

		// Wait for the polling loop to finish
		<-p.stopped
	})
}

// processJobs reads and processes all available jobs from the queue in batches
func (p *CachingQueuePoller) processJobs(ctx context.Context) {
	var wg sync.WaitGroup
	maxGoroutines := make(chan struct{}, maxParallelBatches) // Semaphore to limit concurrent batches

	// Keep processing batches until the queue is empty
	for {
		select {
		case <-ctx.Done():
			return
		case maxGoroutines <- struct{}{}: // Acquire semaphore
		}

		// Read a batch of jobs
		jobs, err := p.queue.ReadJobs(ctx, p.jobBatchSize)
		if err != nil {
			log.Errorf("Error reading jobs from queue: %v", err)
			break
		}

		if len(jobs) == 0 {
			// No more jobs in the queue
			break
		}

		// Process batch in a new goroutine
		wg.Add(1)
		go func(batch []ProviderCachingJob) {
			defer func() {
				// Release semaphore when done
				<-maxGoroutines
				wg.Done()
			}()

			p.processBatch(ctx, batch)
		}(jobs)
	}

	// Wait for all batches to complete
	wg.Wait()
}

// processBatch processes a single batch of jobs in parallel
func (p *CachingQueuePoller) processBatch(ctx context.Context, batch []ProviderCachingJob) {
	var wg sync.WaitGroup

	// Process each job in the batch in parallel
	for _, job := range batch {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// Process the job
			select {
			case <-ctx.Done():
				log.Debug("Context canceled, stopping job processing")
				return
			default:
				if err := p.cacher.CacheProviderForIndexRecords(ctx, job.Provider, job.Index); err != nil {
					log.Errorf("Failed to cache provider %s: %v", job.Provider, err)
					return
				}

				// Delete the job if processing was successful
				if err := p.queue.DeleteJob(ctx, job.ID); err != nil {
					log.Errorf("Failed to delete job %s: %v", job.ID, err)
				}
			}
		}()
	}

	// Wait for all jobs in this batch to complete
	wg.Wait()
}
