package singlewalk

import (
	"context"

	"github.com/storacha-network/indexing-service/pkg/internal/jobwalker"
)

type singleState[State any] struct {
	m State
}

// Access implements jobwalker.WrappedState.
func (s *singleState[State]) Access() State {
	return s.m
}

// CmpSwap implements jobwalker.WrappedState.
func (s *singleState[State]) CmpSwap(willModify func(State) bool, modify func(State) State) bool {
	if !willModify(s.m) {
		return false
	}
	s.m = modify(s.m)
	return true
}

// Modify implements jobwalker.WrappedState.
func (s *singleState[State]) Modify(modify func(State) State) {
	s.m = modify(s.m)
}

var _ jobwalker.WrappedState[any] = &singleState[any]{}

// SingleWalker processes jobs that span more jobs, sequentially depth first in a single thread
func SingleWalker[Job, State any](ctx context.Context, initial []Job, initialState State, handler jobwalker.JobHandler[Job, State]) (State, error) {
	stack := initial
	state := &singleState[State]{initialState}
	for len(stack) > 0 {
		select {
		case <-ctx.Done():
			return state.Access(), ctx.Err()
		default:
		}
		next := initial[len(initial)-1]
		initial = initial[:len(initial)-1]
		handler(ctx, next, func(j Job) error {
			stack = append(stack, j)
			return nil
		}, state)
	}
	return state.Access(), nil
}
