package extmocks

import (
	context "context"

	"github.com/stretchr/testify/mock"
)

// AnyContext is a matcher for testify/mock that matches anything implementing context.Context
var AnyContext = mock.MatchedBy(func(c context.Context) bool {
	// if the passed in parameter does not implement the context.Context interface, the
	// wrapping MatchedBy will panic - so we can simply return true, since we
	// know it's a context.Context if execution flow makes it here.
	return true
})
