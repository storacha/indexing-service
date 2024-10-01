package claimlookup

import (
	"context"
	"errors"
	"net/url"

	"github.com/ipfs/go-cid"
	"github.com/storacha-network/go-ucanto/core/delegation"
	"github.com/storacha-network/indexing-service/pkg/types"
)

type ClaimLookup struct {
	claimStore types.ContentClaimsStore
}

func NewClaimLookup(claimStore types.ContentClaimsStore) *ClaimLookup {
	return &ClaimLookup{
		claimStore: claimStore,
	}
}

func (cl *ClaimLookup) LookupClaim(ctx context.Context, claimCid cid.Cid, fetchURL url.URL) (delegation.Delegation, error) {
	return nil, errors.New("not implemented")
}
