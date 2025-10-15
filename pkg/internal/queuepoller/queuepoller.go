package queuepoller

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

type (
	QueueQueuer[Job any] interface {
		Queue(ctx context.Context, job Job) error
	}

	QueueReader[Job any] interface {
		Read(ctx context.Context, maxJobs int) ([]Job, error)
	}

	QueueReleaser interface {
		Release(ctx context.Context, jobID string) error
	}

	QueueDeleter interface {
		Delete(ctx context.Context, jobID string) error
	}

	Queue[Job any] interface {
		QueueQueuer[Job]
		QueueReader[Job]
		QueueReleaser
		QueueDeleter
	}
)

var log = logging.Logger("queuepoller")

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

type JobHandler[Job any] interface {
	Handle(ctx context.Context, job Job) error
}

type JobIdentifier[Job any] interface {
	ID(job Job) string
}

// QueuePoller polls a queue for  jobs and processes them
// using the provided JobHandler.
type QueuePoller[Job any] struct {
	queue        Queue[Job]
	handler      JobHandler[Job]
	identifier   JobIdentifier[Job]
	jq           *jobqueue.JobQueue[Job]
	jobBatchSize int
	ctx          context.Context
	cancel       context.CancelFunc
	stopped      chan struct{}
	startOnce    sync.Once
	stopOnce     sync.Once
}

// NewQueuePoller creates a new QueuePoller instance.
func NewQueuePoller[Job any](queue Queue[Job], handler JobHandler[Job], identifier JobIdentifier[Job], opts ...Option) (*QueuePoller[Job], error) {
	cfg := &config{
		jobBatchSize: defaultJobBatchSize,
		concurrency:  defaultConcurrency,
	}

	for _, opt := range opts {
		opt(cfg)
	}

	jq := jobqueue.NewJobQueue[Job](
		jobqueue.JobHandler(jobHandler(queue, handler, identifier)),
		jobqueue.WithConcurrency(cfg.concurrency),
		jobqueue.WithErrorHandler(func(err error) {
			log.Errorw("caching provider index", "error", err)
		}))

	poller := &QueuePoller[Job]{
		queue:        queue,
		handler:      handler,
		identifier:   identifier,
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
func (p *QueuePoller[Job]) Start() {
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
func (p *QueuePoller[Job]) Stop() {
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
func (p *QueuePoller[Job]) processJobs(ctx context.Context) {
	// Read a batch of jobs and queue them in the job queue
	jobs, err := p.queue.Read(ctx, p.jobBatchSize)
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

// jobHandler handles a single job
func jobHandler[Job any](queue Queue[Job], handler JobHandler[Job], identifier JobIdentifier[Job]) func(ctx context.Context, job Job) error {
	return func(ctx context.Context, job Job) error {
		jobCtx, cancel := context.WithTimeout(ctx, maxJobProcessingTime)
		defer cancel()

		// Process the job
		err := handler.Handle(jobCtx, job)
		if err != nil && !errors.Is(err, context.DeadlineExceeded) {
			// if the error is not a timeout, make the job visible so that it can be retried
			if err := queue.Release(ctx, identifier.ID(job)); err != nil {
				log.Warnf("Failed to release job %s: %s", identifier.ID(job), err)
			}

			return fmt.Errorf("failed to perform job %s: %w", identifier.ID(job), err)
		}

		// Do not hold up the queue by re-attempting a cache job that times out. It is
		// probably a big DAG and retrying is unlikely to subsequently succeed.
		// Log the error and proceed with deletion.
		if errors.Is(err, context.DeadlineExceeded) {
			log.Warnf("Not retrying provider job for %s: %s", identifier.ID(job), err)
		}

		// Delete the job too if processing was successful
		if err := queue.Delete(ctx, identifier.ID(job)); err != nil {
			return fmt.Errorf("failed to delete job %s: %w", identifier.ID(job), err)
		}

		return nil
	}
}
