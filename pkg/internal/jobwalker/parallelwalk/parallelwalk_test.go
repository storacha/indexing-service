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

	initialJobs := []int{1, 2, 3, 4, 5}

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

	// happy path - all jobs, including spawned jobs, are processed as expected
	t.Run("happy path", func(t *testing.T) {
		state, err := parallelWalk(context.Background(), initialJobs, []int{}, handler)

		assert.NoError(t, err)
		assert.Equal(t, 11, len(state))
		for _, job := range []int{1, 2, 3, 4, 5, 12, 22, 32, 14, 24, 34} {
			assert.Contains(t, state, job)
		}
	})

	// cancels as expected when the context is cancelled
	t.Run("context cancelation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := parallelWalk(ctx, initialJobs, []int{}, handler)
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	// returns err when the handler errors
	t.Run("handler error", func(t *testing.T) {
		handler := func(ctx context.Context, j int, spawn func(int) error, state jobwalker.WrappedState[[]int]) error {
			return errors.New("test error")
		}
		_, err := parallelWalk(context.Background(), initialJobs, []int{}, handler)
		assert.Error(t, err)
		assert.Equal(t, "test error", err.Error())
	})
}
