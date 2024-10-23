package claimlookup

import (
	"context"
	"net/url"

	"github.com/ipfs/go-cid"
	"github.com/storacha/go-ucanto/core/delegation"
)

// ClaimLookup is used to get full claims from a claim cid
type ClaimLookup interface {
	// LookupClaim should:
	// 1. attempt to read the claim from the cache from the encoded contextID
	// 2. if not found, attempt to fetch the claim from the provided URL. Store the result in cache
	// 3. return the claim
	LookupClaim(ctx context.Context, claimCid cid.Cid, fetchURL url.URL) (delegation.Delegation, error)
}

type ClaimCacher interface {
	// CacheClaim writes the passed claim to the cache with default expiry.
	CacheClaim(ctx context.Context, claim delegation.Delegation) error
}
