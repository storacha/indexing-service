package providercacher

import (
	"context"
	"errors"
	"sync"

	"github.com/ipni/go-libipni/find/model"
	"github.com/storacha/indexing-service/pkg/blobindex"
)

var ErrQueueShutdown = errors.New("queue is shutdown")

type (
	cacheProviderJob struct {
		provider   model.ProviderResult
		index      blobindex.ShardedDagIndexView
		returnChan chan returnValue
	}

	returnValue struct {
		written uint64
		err     error
	}

	ParallelProviderCacher struct {
		*config
		cacher   ProviderCacher
		incoming chan cacheProviderJob
		closing  chan struct{}
		closed   chan struct{}
	}

	config struct {
		buffer      int
		concurrency int
	}

	// Option modifies a ParallelProviderCacher
	Option func(*config)
)

func WithBuffer(buffer int) Option {
	return func(c *config) {
		c.buffer = buffer
	}
}

func WithConcurrency(concurrency int) Option {
	return func(c *config) {
		c.concurrency = concurrency
	}
}

func NewParallelProviderCacher(cacher ProviderCacher, opts ...Option) *ParallelProviderCacher {
	c := &config{
		buffer:      0,
		concurrency: 1,
	}
	for _, opt := range opts {
		opt(c)
	}
	return &ParallelProviderCacher{
		config:   c,
		cacher:   cacher,
		incoming: make(chan cacheProviderJob, c.buffer),
		closing:  make(chan struct{}),
		closed:   make(chan struct{}),
	}
}

func (p *ParallelProviderCacher) CacheProviderForIndexRecords(ctx context.Context, provider model.ProviderResult, index blobindex.ShardedDagIndexView) (uint64, error) {
	returnChan := make(chan returnValue, 1)
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-p.closing:
	case p.incoming <- cacheProviderJob{provider, index, returnChan}:
	}

	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-p.closed:
		return 0, ErrQueueShutdown
	case r := <-returnChan:
		return r.written, r.err
	}
}

func (p *ParallelProviderCacher) Startup() {
	defer close(p.closed)
	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())
	for range p.concurrency {
		wg.Add(1)
		go func() {
			p.worker(ctx)
			wg.Done()
		}()
	}
	wg.Add(1)
	go func() {
		<-p.closing
		cancel()
		wg.Done()
	}()
	wg.Wait()
}

func (p *ParallelProviderCacher) Shutdown(ctx context.Context) error {
	close(p.closing)
	select {
	case <-p.closed:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *ParallelProviderCacher) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-p.incoming:
			written, err := p.cacher.CacheProviderForIndexRecords(ctx, job.provider, job.index)
			job.returnChan <- returnValue{written, err}
		}
	}
}
