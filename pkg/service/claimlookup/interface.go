package claimlookup

import (
	"context"
	"net/url"

	"github.com/ipfs/go-cid"
	"github.com/storacha/go-ucanto/core/delegation"
)

type ClaimLookup interface {
	LookupClaim(ctx context.Context, claimCid cid.Cid, fetchURL url.URL) (delegation.Delegation, error)
}
