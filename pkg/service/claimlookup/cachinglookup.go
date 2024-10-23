package claimlookup

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/ipfs/go-cid"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/indexing-service/pkg/internal/link"
	"github.com/storacha/indexing-service/pkg/types"
)

type cachingLookup struct {
	claimLookup ClaimLookup
	claimStore  types.ContentClaimsStore
}

var _ ClaimLookup = (*cachingLookup)(nil)
var _ ClaimCacher = (*cachingLookup)(nil)

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
	err = cacheClaim(ctx, cl.claimStore, claim)
	if err != nil {
		return nil, fmt.Errorf("caching claim: %w", err)
	}
	return claim, nil
}

func (cl *cachingLookup) CacheClaim(ctx context.Context, claim delegation.Delegation) error {
	err := cacheClaim(ctx, cl.claimStore, claim)
	if err != nil {
		return fmt.Errorf("caching claim: %w", err)
	}
	if cacher, ok := cl.claimLookup.(ClaimCacher); ok {
		// cache with underlying claim lookup
		err = cacher.CacheClaim(ctx, claim)
		if err != nil {
			return fmt.Errorf("caching claim in underlying cacher: %w", err)
		}
	}
	return nil
}

func cacheClaim(ctx context.Context, claimStore types.ContentClaimsStore, claim delegation.Delegation) error {
	return claimStore.Set(ctx, link.ToCID(claim.Link()), claim, true)
}
