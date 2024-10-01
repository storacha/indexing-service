package parallelwalk

import (
	"context"
	"errors"
	"sync"
)

// JobHandler handles the specified job and uses the passed in function to spawn more
// jobs.
// The handler should stop processing if spawn errors, returning the error from spawn
type JobHandler[Job any, State any] func(ctx context.Context, j Job, spawn func(Job) error, stateModifier func(func(State) State)) error

// ParallelWalk is a function to handle a series of jobs that may spawn more jobs
// It will execute jobs in parallel, with the specified concurrency until all initial jobs
// and all spawned jobs (recursively) are handled, or a job errors
// This code is adapted from https://github.com/ipfs/go-merkledag/blob/master/merkledag.go#L464C6-L584
func ParallelWalk[Job, State any](ctx context.Context, initial []Job, state State, handler JobHandler[Job, State], concurrency int) (State, error) {
	if len(initial) == 0 {
		return state, errors.New("must provide at least one initial job")
	}
	jobFeed := make(chan Job)
	spawnedJobs := make(chan Job)
	jobFinishes := make(chan struct{})

	var stateLk sync.Mutex

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
				}, func(modifyState func(State) State) {
					stateLk.Lock()
					state = modifyState(state)
					stateLk.Unlock()
				})

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
				return state, nil
			}
		case queued := <-spawnedJobs:
			if !hasNextJob() {
				nextJob = queued
				jobProcessor = jobFeed
			} else {
				queuedJobs = append(queuedJobs, queued)
			}
		case err := <-errChan:
			return state, err
		case <-ctx.Done():
			return state, ctx.Err()
		}
	}
}
