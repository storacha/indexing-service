package contentclaims

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/ipld/go-ipld-prime"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/indexing-service/pkg/types"
)

type storeFinder struct {
	finder Finder
	store  types.ContentClaimsStore
}

var _ Finder = (*cachingFinder)(nil)

// WithStore augments a ClaimFinder with claims retrieved from a claim store
func WithStore(finder Finder, store types.ContentClaimsStore) Finder {
	return &storeFinder{finder, store}
}

// Find attempts to fetch a claim from either the permenant storage or via the provided URL
func (sf *storeFinder) Find(ctx context.Context, id ipld.Link, fetchURL *url.URL) (delegation.Delegation, error) {
	// attempt to read claim from store and return it if succesful
	claim, err := sf.store.Get(ctx, id)
	if err == nil {
		return claim, nil
	}

	// if an error occurred other than the claim not being in the store, return it
	if !errors.Is(err, types.ErrKeyNotFound) {
		return nil, fmt.Errorf("reading from claim store: %w", err)
	}

	// attempt to fetch the claim from the underlying claim finder
	return sf.finder.Find(ctx, id, fetchURL)
}
