package jobqueue

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ErrQueueShutdown means the queue is shutdown so the job could not be queued
var ErrQueueShutdown = errors.New("queue is shutdown")

type (
	// Option modifies the config of a JobQueue
	Option func(*config)

	// Handler handles jobs of the given type
	Handler[Job any] func(ctx context.Context, j Job) error

	// JobQueue is asyncronous queue for jobs, that can be processed in parallel
	// by the job queue's handler
	JobQueue[Job any] struct {
		*config
		handler  Handler[Job]
		incoming chan quitOrJob
		closed   chan struct{}
		closing  chan struct{}
	}

	config struct {
		jobTimeout      time.Duration
		shutdownTimeout time.Duration
		errorHandler    func(error)
		buffer          int
		concurrency     int
	}

	quitOrJob interface {
		isQuitOrJob()
	}

	job[Job any] struct {
		j Job
	}
	quit struct{}
)

// WithBuffer allows a set amount of jobs to be buffered even if all workers are busy
func WithBuffer(buffer int) Option {
	return func(c *config) {
		c.buffer = buffer
	}
}

// WithConcurrency sets the number of workers that will process jobs in
// parallel
func WithConcurrency(concurrency int) Option {
	return func(c *config) {
		c.concurrency = concurrency
	}
}

// WithErrorHandler uses the given error handler whenever a job errors while processing
func WithErrorHandler(errorHandler func(error)) Option {
	return func(c *config) {
		c.errorHandler = errorHandler
	}
}

// WithJobTimeout cancels the past into context to the job handler after the specified
// timeout
func WithJobTimeout(jobTimeout time.Duration) Option {
	return func(c *config) {
		c.jobTimeout = jobTimeout
	}
}

// WithShutdownTimeout sets the shutdown timeout. When the queue is shutdown, the
// context passed to all job handlers will cancel after the specified timeout
func WithShutdownTimeout(shutdownTimeout time.Duration) Option {
	return func(c *config) {
		c.shutdownTimeout = shutdownTimeout
	}
}

// NewJobQueue returns a new job queue that processes with the given handler
func NewJobQueue[Job any](handler Handler[Job], opts ...Option) *JobQueue[Job] {
	c := &config{
		concurrency: 1,
	}
	for _, opt := range opts {
		opt(c)
	}
	return &JobQueue[Job]{
		config:   c,
		handler:  handler,
		incoming: make(chan quitOrJob),
		closing:  make(chan struct{}),
		closed:   make(chan struct{}),
	}
}

// Queue attempts to queue the job. It will fail if the queue is shutdown, or
// the passed context cancels before the job can be queued
func (p *JobQueue[Job]) Queue(ctx context.Context, j Job) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-p.closing:
		return ErrQueueShutdown
	case p.incoming <- job[Job]{j}:
		return nil
	}
}

// Startup starts the queue in the background (returns immediately)
func (p *JobQueue[Job]) Startup() {
	go p.run()
}

// Shutdown shuts down the queue, returning when the whole queue is shutdown or
// the passed context cancels
func (p *JobQueue[Job]) Shutdown(ctx context.Context) error {
	// signal the queue is closing -- this will cause anyone awaiting a queue
	// to abort
	close(p.closing)
	// now get get a quit message into the incoming queue this will be the last
	// message written in the queue but we also don't just close incoming cause it
	// would cause a potential panic
	p.incoming <- quit{}
	// now wait for the go routines to complete
	select {
	case <-p.closed:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *JobQueue[Job]) run() {
	// the queue is fully closed when this function completes
	defer close(p.closed)
	// outgoing will be used to consume jobs by the workers
	outgoing := make(chan Job, p.buffer)

	// setup a cancellable context so that you can shut down all job
	// executions when ready
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	// spin up all workers
	for range p.concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.worker(ctx, outgoing)
		}()
	}

	for {
		// read the next message from the incoming queue
		queued := <-p.incoming
		switch typed := queued.(type) {
		case job[Job]:
			// if its a job, just send to the workers
			outgoing <- typed.j
		case quit:
			// if it's a quit message, this is the last message we will receive
			// so start the shutdown process

			// tell all the workers they're done processing jobs
			close(outgoing)
			// if there is a shut down timeout, queue a background routune to cancel
			// the context (i.e. accelerate workers shutting down by the handler getting
			// a shutdown context)
			if p.shutdownTimeout != 0 {
				timer := time.NewTimer(p.shutdownTimeout)
				go func() {
					<-timer.C
					cancel()
				}()
			}
			// wait for the workers to shutdown
			wg.Wait()
			return
		}
	}
}

func (p *JobQueue[Job]) jobCtx(ctx context.Context) (context.Context, context.CancelFunc) {
	if p.jobTimeout != 0 {
		return context.WithTimeout(ctx, p.jobTimeout)
	}
	return context.WithCancel(ctx)
}

func (p *JobQueue[Job]) handleJob(ctx context.Context, job Job) {
	ctx, cancel := p.jobCtx(ctx)
	defer cancel()
	err := p.handler(ctx, job)
	if err != nil && p.errorHandler != nil {
		p.errorHandler(err)
	}
}

func (p *JobQueue[Job]) worker(ctx context.Context, jobs <-chan Job) {
	for job := range jobs {
		p.handleJob(ctx, job)
	}
}

func (job[Job]) isQuitOrJob() {}
func (quit) isQuitOrJob()     {}
