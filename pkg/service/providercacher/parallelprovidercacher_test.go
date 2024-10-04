package providercacher_test

/*
import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ipni/go-libipni/find/model"
	"github.com/storacha/indexing-service/pkg/blobindex"
	"github.com/storacha/indexing-service/pkg/internal/testutil"
	"github.com/storacha/indexing-service/pkg/service/providercacher"
)

func TestParallelProviderCacher_CacheProviderForIndexRecords(t *testing.T) {
	testProvider := testutil.RandomProviderResult()
	index := blobindex.NewShardedDagIndexView(testutil.RandomCID(), 0)

}

type result struct {
	written uint64
	err     error
}

type jobResult struct {
	result
	jobIndex int
}

type state struct {
	*mockBlockingCacher
	jobCancellers []context.CancelFunc
	runCanceller  context.CancelFunc
	jobResults    <-chan jobResult
	wg            sync.WaitGroup
	jobCount      int
}

func startup(ctx context.Context, buffer int, concurrency int, jobCount int, defaultWritten uint64, defaultErr error) *state {
	provider := testutil.RandomProviderResult()
	index := blobindex.NewShardedDagIndexView(testutil.RandomCID(), 0)
	js := make(chan jobResult, jobCount)
	s := &state{
		mockBlockingCacher: &mockBlockingCacher{
			written: defaultWritten,
			err:     defaultErr,
			blocker: make(chan struct{}),
		},
		jobCancellers: make([]context.CancelFunc, 0, jobCount),
		jobResults:    js,
		jobCount:      jobCount,
	}

	providerCacher := providercacher.NewParallelProviderCacher(s.mockBlockingCacher, providercacher.WithBuffer(buffer), providercacher.WithConcurrency(concurrency))
	for i := range jobCount {
		jctx, jcancel := context.WithCancel(ctx)
		s.jobCancellers = append(s.jobCancellers, jcancel)
		s.wg.Add(1)
		go func(jctx context.Context, i int) {
			defer s.wg.Done()
			written, err := providerCacher.CacheProviderForIndexRecords(jctx, provider, index)
			js <- jobResult{result: result{written, err}, jobIndex: i}
		}(jctx, i)
	}
	rctx, rcancel := context.WithCancel(ctx)
	s.runCanceller = rcancel
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		providerCacher.Run(rctx)
	}()
	return s
}

func (s *state) cancelJob(index int) {
	s.jobCancellers[index]()
}

func (s *state) closeAndGetResults() []result {
	results := make([]result, s.jobCount)
	for i := range s.jobCount {

	}
}

type mockBlockingCacher struct {
	written uint64
	err     error

	countJobs atomic.Uint64
	blocker   chan struct{}
}

func (m *mockBlockingCacher) advanceOne() {
	m.blocker <- struct{}{}
}

// CacheProviderForIndexRecords implements providercacher.ProviderCacher.
func (m *mockBlockingCacher) CacheProviderForIndexRecords(ctx context.Context, provider model.ProviderResult, index blobindex.ShardedDagIndexView) (written uint64, err error) {
	m.countJobs.Add(1)
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-m.blocker:
		return m.written, m.err
	}
}

var _ providercacher.ProviderCacher = mockBlockingCacher{}
*/
