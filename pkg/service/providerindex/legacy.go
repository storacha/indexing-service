package providerindex

import (
	"context"

	"github.com/ipfs/go-cid"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/ipni/go-libipni/find/model"
	"github.com/multiformats/go-multihash"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/indexing-service/pkg/types"
)

// LegacyClaimsFinder is a read-only interface to find claims on a legacy system
type LegacyClaimsFinder interface {
	Find(ctx context.Context, contentHash multihash.Multihash) ([]model.ProviderResult, error)
}

// LegacyClaimsStore allows finding claims on a legacy store
type LegacyClaimsStore struct {
	contentToClaims ContentToClaimsMapper
	claimsStore     types.ContentClaimsStore
}

// ContentToClaimsMapper maps content hashes to claim cids
type ContentToClaimsMapper interface {
	GetClaims(ctx context.Context, contentHash multihash.Multihash) (claimsCids []cid.Cid, err error)
}

func NewLegacyClaimsStore(contentToClaimsMapper ContentToClaimsMapper, claimStore types.ContentClaimsStore) LegacyClaimsStore {
	return LegacyClaimsStore{
		contentToClaims: contentToClaimsMapper,
		claimsStore:     claimStore,
	}
}

// Find looks for the corresponding claims for a given content hash in the mapper and then fetches the claims from the
// claims store
func (ls LegacyClaimsStore) Find(ctx context.Context, contentHash multihash.Multihash) ([]model.ProviderResult, error) {
	claimsCids, err := ls.contentToClaims.GetClaims(ctx, contentHash)
	if err != nil {
		return nil, err
	}

	results := []model.ProviderResult{}

	for _, claimCid := range claimsCids {
		claim, err := ls.claimsStore.Get(ctx, cidlink.Link{Cid: claimCid})
		if err != nil {
			return nil, err
		}

		pr, err := synthetizeProviderResult(claim)
		if err != nil {
			return nil, err
		}

		results = append(results, pr)
	}

	return results, nil
}

// synthetizeProviderResult synthetizes a provider result, including metadata, from a given claim
func synthetizeProviderResult(_ delegation.Delegation) (model.ProviderResult, error) {
	// TODO: implement
	return model.ProviderResult{}, nil
}

// NotFoundLegacyClaimsFinder is a LegacyClaimsFinder that always returns ErrKeyNotFound. It can be used when accessing
// claims in a legacy system is not required
type NotFoundLegacyClaimsFinder struct{}

func NewNotFoundLegacyClaimsFinder() NotFoundLegacyClaimsFinder {
	return NotFoundLegacyClaimsFinder{}
}

// Find always returns ErrKeyNotFound
func (f NotFoundLegacyClaimsFinder) Find(ctx context.Context, contentHash multihash.Multihash) ([]model.ProviderResult, error) {
	return nil, types.ErrKeyNotFound
}
