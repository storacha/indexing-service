package contentclaims

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/ipld/go-ipld-prime"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/go-ucanto/core/result"
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

	// buffered channels so goroutines don't block.
	storeCh := make(chan result.Result[delegation.Delegation, error], 1)
	finderCh := make(chan result.Result[delegation.Delegation, error], 1)

	// Create a cancelable context for the store query.
	storeCtx, cancelStore := context.WithCancel(ctx)
	defer cancelStore()

	// Create a cancelable context for the finder query.
	finderCtx, cancelFinder := context.WithCancel(ctx)
	defer cancelFinder()

	// Start store query
	go func() {
		storeCh <- result.Wrap(func() (delegation.Delegation, error) { return sf.store.Get(storeCtx, id) })
	}()

	// Start finder query
	go func() {
		finderCh <- result.Wrap(func() (delegation.Delegation, error) { return sf.finder.Find(finderCtx, id, fetchURL) })
	}()

	var storeRes, finderRes result.Result[delegation.Delegation, error]

	// Wait for both responses.
	for range 2 {
		select {
		case storeRes = <-storeCh:
			if _, err := result.Unwrap(storeRes); err == nil {
				cancelFinder()
			}
		case finderRes = <-finderCh:
			if _, err := result.Unwrap(finderRes); err == nil {
				cancelStore()
			}
		}
	}

	// Return any successful result or the combo of errors
	return result.Unwrap(
		result.OrElse(storeRes,
			func(storeErr error) result.Result[delegation.Delegation, error] {
				return result.OrElse(finderRes, func(finderErr error) result.Result[delegation.Delegation, error] {
					// if an error occurred other than the claim not being in the store, return it
					if !errors.Is(storeErr, types.ErrKeyNotFound) {
						return result.Error[delegation.Delegation](errors.Join(fmt.Errorf("reading from claim store: %w", storeErr), finderErr))
					}
					return result.Error[delegation.Delegation](finderErr)
				})
			}),
	)
}
