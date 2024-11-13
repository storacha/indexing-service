package parallelwalk

import (
	"context"
	"errors"
	"testing"

	"github.com/storacha/indexing-service/pkg/internal/jobwalker"
	"github.com/stretchr/testify/assert"
)

func TestParallelWalk(t *testing.T) {
	concurrency := 3
	var parallelWalk = NewParallelWalk[int, []int](concurrency)

	// happy path - all jobs, including spawned jobs, are processed as expected
	handler := func(ctx context.Context, j int, spawn func(int) error, state jobwalker.WrappedState[[]int]) error {
		state.Modify(func(ints []int) []int {
			return append(ints, j)
		})

		// jobs 2 and 4 spawn 3 new jobs each
		if j == 2 || j == 4 {
			if err := spawn(10 + j); err != nil {
				return err
			}
			if err := spawn(20 + j); err != nil {
				return err
			}
			if err := spawn(30 + j); err != nil {
				return err
			}
		}

		return nil
	}

	initialJobs := []int{1, 2, 3, 4, 5}

	state, err := parallelWalk(context.Background(), initialJobs, []int{}, handler)

	assert.NoError(t, err)
	assert.Equal(t, 11, len(state))
	for _, job := range []int{1, 2, 3, 4, 5, 12, 22, 32, 14, 24, 34} {
		assert.Contains(t, state, job)
	}

	// cancels as expected when the context is cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	state, err = parallelWalk(ctx, initialJobs, []int{}, handler)
	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)

	// returns err when the handler errors
	handler = func(ctx context.Context, j int, spawn func(int) error, state jobwalker.WrappedState[[]int]) error {
		return errors.New("test error")
	}
	_, err = parallelWalk(context.Background(), initialJobs, []int{}, handler)
	assert.Error(t, err)
	assert.Equal(t, "test error", err.Error())
}
