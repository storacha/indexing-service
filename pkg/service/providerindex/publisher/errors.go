package publisher

import "errors"

var (

	// ErrContextIDNotFound signals that no item is associated to the given context ID.
	ErrContextIDNotFound = errors.New("context ID not found")

	// ErrAlreadyAdvertised signals that an advertisement for identical content was already
	// published.
	ErrAlreadyAdvertised = errors.New("advertisement already published")
)
