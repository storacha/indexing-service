package contentclaims

import (
	"context"
	"net/url"

	"github.com/ipld/go-ipld-prime/datamodel"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/indexing-service/pkg/types"
)

type (
	notFoundFinder struct{}
)

var _ Finder = notFoundFinder{}

// NewNotFoundFinder returns a finder that always errors
func NewNotFoundFinder() Finder {
	return notFoundFinder{}
}

// Find implements Finder.
func (n notFoundFinder) Find(ctx context.Context, claim datamodel.Link, fetchURL url.URL) (delegation.Delegation, error) {
	return nil, types.ErrKeyNotFound
}
