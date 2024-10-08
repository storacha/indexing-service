package claimlookup

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/ipfs/go-cid"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/indexing-service/pkg/types"
)

type cachingLookup struct {
	claimLookup ClaimLookup
	claimStore  types.ContentClaimsStore
}

// WithCache augments a ClaimLookup with cached claims from a claim store
func WithCache(claimLookup ClaimLookup, claimStore types.ContentClaimsStore) ClaimLookup {
	return &cachingLookup{
		claimLookup: claimLookup,
		claimStore:  claimStore,
	}
}

// LookupClaim attempts to fetch a claim from either the local cache or via the provided URL (caching the result if its fetched)
func (cl *cachingLookup) LookupClaim(ctx context.Context, claimCid cid.Cid, fetchURL url.URL) (delegation.Delegation, error) {
	// attempt to read claim from cache and return it if succesful
	claim, err := cl.claimStore.Get(ctx, claimCid)
	if err == nil {
		return claim, nil
	}

	// if an error occurred other than the claim not being in the cache, return it
	if !errors.Is(err, types.ErrKeyNotFound) {
		return nil, fmt.Errorf("reading from claim cache: %w", err)
	}

	// attempt to fetch the claim from the underlying claim lookup
	claim, err = cl.claimLookup.LookupClaim(ctx, claimCid, fetchURL)
	if err != nil {
		return nil, fmt.Errorf("fetching underlying claim: %w", err)
	}

	// cache the claim for the future
	if err := cl.claimStore.Set(ctx, claimCid, claim, true); err != nil {
		return nil, fmt.Errorf("caching fetched claim: %w", err)
	}
	return claim, nil
}
