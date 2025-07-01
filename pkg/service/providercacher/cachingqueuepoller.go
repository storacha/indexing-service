package providercacher

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	logging "github.com/ipfs/go-log/v2"
	"github.com/storacha/go-libstoracha/jobqueue"
)

const (
	defaultJobBatchSize = 10
	defaultConcurrency  = 100

	maxJobBatchSize      = 10
	maxJobProcessingTime = 5 * time.Minute
)

var log = logging.Logger("service/providercacher")

// config
type config struct {
	jobBatchSize int
	concurrency  int
}

// Option configures the CachingQueuePoller
type Option func(*config)

// WithJobBatchSize sets the maximum number of jobs to process in a batch
func WithJobBatchSize(size int) Option {
	return func(cfg *config) {
		cfg.jobBatchSize = size
	}
}

// WithConcurrency sets the maximum number of concurrent job processing
func WithConcurrency(concurrency int) Option {
	return func(cfg *config) {
		cfg.concurrency = concurrency
	}
}

// CachingQueuePoller polls a queue for provider caching jobs and processes them
// using the provided ProviderCacher and SQSCachingDecoder.
type CachingQueuePoller struct {
	queue        CachingQueue
	cacher       ProviderCacher
	jq           *jobqueue.JobQueue[ProviderCachingJob]
	jobBatchSize int
	ctx          context.Context
	cancel       context.CancelFunc
	stopped      chan struct{}
	startOnce    sync.Once
	stopOnce     sync.Once
}

// NewCachingQueuePoller creates a new CachingQueuePoller instance.
func NewCachingQueuePoller(queue CachingQueue, cacher ProviderCacher, opts ...Option) (*CachingQueuePoller, error) {
	cfg := &config{
		jobBatchSize: defaultJobBatchSize,
		concurrency:  defaultConcurrency,
	}

	for _, opt := range opts {
		opt(cfg)
	}

	jq := jobqueue.NewJobQueue[ProviderCachingJob](
		jobqueue.JobHandler(cachingJobHandler(queue, cacher)),
		jobqueue.WithConcurrency(cfg.concurrency),
		jobqueue.WithErrorHandler(func(err error) {
			log.Errorw("caching provider index", "error", err)
		}))

	poller := &CachingQueuePoller{
		queue:        queue,
		cacher:       cacher,
		jq:           jq,
		jobBatchSize: cfg.jobBatchSize,
		stopped:      make(chan struct{}),
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
		p.jq.Startup()
		log.Info("Starting caching queue poller")

		go func() {
			for {
				select {
				case <-p.ctx.Done():
					log.Info("Stopping polling loop")
					close(p.stopped)
					return
				default:
					p.processJobs(p.ctx)
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

		p.jq.Shutdown(p.ctx)
	})
}

// processJobs reads and processes all available jobs from the queue in batches
func (p *CachingQueuePoller) processJobs(ctx context.Context) {
	// Read a batch of jobs and queue them in the job queue
	jobs, err := p.queue.ReadJobs(ctx, p.jobBatchSize)
	if err != nil {
		log.Errorf("Error reading jobs from queue: %v", err)
		return
	}

	for _, job := range jobs {
		err := p.jq.Queue(ctx, job)
		if err != nil {
			log.Errorf("Error queuing job: %v", err)
		}
	}
}

// cachingJobHandler handles a single job
func cachingJobHandler(queue CachingQueue, cacher ProviderCacher) func(ctx context.Context, job ProviderCachingJob) error {
	return func(ctx context.Context, job ProviderCachingJob) error {
		jobCtx, cancel := context.WithTimeout(ctx, maxJobProcessingTime)
		defer cancel()

		// Process the job
		err := cacher.CacheProviderForIndexRecords(jobCtx, job.Provider, job.Index)
		if err != nil && !errors.Is(err, context.DeadlineExceeded) {
			// if the error is not a timeout, make the job visible so that it can be retried
			if err := queue.Release(ctx, job.ID); err != nil {
				log.Warnf("Failed to release job %s: %s", job.ID, err)
			}

			return fmt.Errorf("failed to cache provider %s: %w", job.Provider, err)
		}

		// Do not hold up the queue by re-attempting a cache job that times out. It is
		// probably a big DAG and retrying is unlikely to subsequently succeed.
		// Log the error and proceed with deletion.
		if errors.Is(err, context.DeadlineExceeded) {
			log.Warnf("Not retrying cache provider job for %s: %s", job.Index.Content(), err)
		}

		// Delete the job too if processing was successful
		if err := queue.DeleteJob(ctx, job.ID); err != nil {
			return fmt.Errorf("failed to delete job %s: %w", job.ID, err)
		}

		return nil
	}
}
