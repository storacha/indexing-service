package providerindex

import (
	"context"

	"github.com/ipfs/go-cid"
	"github.com/ipni/go-libipni/find/model"
	"github.com/multiformats/go-multihash"
	"github.com/storacha/go-ucanto/core/delegation"
)

// LegacyClaimsStore is a read-only interface to a legacy claims store
type LegacyClaimsStore struct {
	contentToClaim ContentToClaimMapper
	claimsStore    ClaimsStore
}

type ContentToClaimMapper interface {
	GetClaim(ctx context.Context, contentHash multihash.Multihash) (claimCid cid.Cid, err error)
}

type ClaimsStore interface {
	GetClaim(ctx context.Context, claimCid cid.Cid) (claim delegation.Delegation, err error)
}

func NewLegacyStore(contentToClaimMapper ContentToClaimMapper, claimStore ClaimsStore) LegacyClaimsStore {
	return LegacyClaimsStore{
		contentToClaim: contentToClaimMapper,
		claimsStore:    claimStore,
	}
}

func (ls LegacyClaimsStore) Find(ctx context.Context, contentHash multihash.Multihash) ([]model.ProviderResult, error) {
	claimCid, err := ls.contentToClaim.GetClaim(ctx, contentHash)
	if err != nil {
		return nil, err
	}

	claim, err := ls.claimsStore.GetClaim(ctx, claimCid)
	if err != nil {
		return nil, err
	}

	return claimsToProviderResults(claim)
}

// claimsToProviderResults synthetizes provider results from a set of claims
// TODO: implement
func claimsToProviderResults(_ delegation.Delegation) ([]model.ProviderResult, error) {
	return []model.ProviderResult{}, nil
}
