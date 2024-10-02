package parallelwalk

import (
	"context"
	"errors"
	"sync"

	"github.com/storacha-network/indexing-service/pkg/internal/jobwalker"
)

type threadSafeState[State any] struct {
	state State
	lk    sync.RWMutex
}

func (ts *threadSafeState[State]) Access() State {
	ts.lk.RLock()
	defer ts.lk.RUnlock()
	return ts.state
}

func (ts *threadSafeState[State]) Modify(modify func(State) State) {
	ts.lk.Lock()
	defer ts.lk.Unlock()
	ts.modify(modify)
}

func (ts *threadSafeState[State]) modify(modify func(State) State) {
	ts.state = modify(ts.state)
}

func (ts *threadSafeState[State]) CmpSwap(willModify func(State) bool, modify func(State) State) bool {
	if !willModify(ts.Access()) {
		return false
	}
	ts.lk.Lock()
	defer ts.lk.Unlock()
	if !willModify(ts.state) {
		return false
	}
	ts.modify(modify)
	return true
}

// NewParallelWalk generates a function to handle a series of jobs that may spawn more jobs
// It will execute jobs in parallel, with the specified concurrency until all initial jobs
// and all spawned jobs (recursively) are handled, or a job errors
// This code is adapted from https://github.com/ipfs/go-merkledag/blob/master/merkledag.go#L464C6-L584
func NewParallelWalk[Job, State any](concurrency int) jobwalker.JobWalker[Job, State] {
	return func(ctx context.Context, initial []Job, initialState State, handler jobwalker.JobHandler[Job, State]) (State, error) {
		if len(initial) == 0 {
			return initialState, errors.New("must provide at least one initial job")
		}
		jobFeed := make(chan Job)
		spawnedJobs := make(chan Job)
		jobFinishes := make(chan struct{})

		state := &threadSafeState[State]{
			state: initialState,
		}
		var wg sync.WaitGroup

		errChan := make(chan error)

		jobFeedCtx, cancel := context.WithCancel(ctx)

		defer wg.Wait()
		defer cancel()
		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for job := range jobFeed {

					err := handler(jobFeedCtx, job, func(next Job) error {
						select {
						case spawnedJobs <- next:
							return nil
						case <-jobFeedCtx.Done():
							return jobFeedCtx.Err()
						}
					}, state)

					if err != nil {
						select {
						case errChan <- err:
						case <-jobFeedCtx.Done():
						}
						return
					}

					select {
					case jobFinishes <- struct{}{}:
					case <-jobFeedCtx.Done():
					}
				}
			}()
		}
		defer close(jobFeed)

		jobProcessor := jobFeed
		nextJob, queuedJobs := initial[0], initial[1:]

		var inProgress int

		hasNextJob := func() bool {
			return jobProcessor != nil
		}

		for {
			select {
			case jobProcessor <- nextJob:
				inProgress++
				if len(queuedJobs) > 0 {
					nextJob = queuedJobs[0]
					queuedJobs = queuedJobs[1:]
				} else {
					var empty Job
					nextJob = empty
					jobProcessor = nil
				}
			case <-jobFinishes:
				inProgress--
				if inProgress == 0 && !hasNextJob() {
					return state.Access(), nil
				}
			case queued := <-spawnedJobs:
				if !hasNextJob() {
					nextJob = queued
					jobProcessor = jobFeed
				} else {
					queuedJobs = append(queuedJobs, queued)
				}
			case err := <-errChan:
				return state.Access(), err
			case <-ctx.Done():
				return state.Access(), ctx.Err()
			}
		}
	}
}
