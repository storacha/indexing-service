package jobwalker

import "context"

// WrappedState is a wrapper around any state to enable atomic access and modification
type WrappedState[State any] interface {
	Access() State
	Modify(func(State) State)
	// CmpSwap calls the "willModify" function (potentially multiple times) and calls modify if it returns true
	CmpSwap(willModify func(State) bool, modify func(State) State) bool
}

// JobHandler handles the specified job and uses the passed in function to spawn more
// jobs.
// The handler should stop processing if spawn errors, returning the error from spawn
type JobHandler[Job any, State any] func(ctx context.Context, j Job, spawn func(Job) error, state WrappedState[State]) error

// JobWalker processes a set of jobs that spawn other jobs, all while modifying a final state
type JobWalker[Job, State any] func(ctx context.Context, initial []Job, initialState State, handler JobHandler[Job, State]) (State, error)
