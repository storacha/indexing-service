package providerindex

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/storacha/go-capabilities/pkg/assert"
	"github.com/storacha/go-metadata"
	"github.com/storacha/indexing-service/pkg/internal/digestutil"
	"github.com/storacha/indexing-service/pkg/types"

	"github.com/ipfs/go-cid"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/ipni/go-libipni/find/model"
	"github.com/ipni/go-libipni/maurl"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/multiformats/go-multihash"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/go-ucanto/did"
)

// LegacyClaimsFinder is a read-only interface to find claims on a legacy system
type LegacyClaimsFinder interface {
	Find(ctx context.Context, contentHash multihash.Multihash) ([]model.ProviderResult, error)
}

// LegacyClaimsStore allows finding claims on a legacy store
type LegacyClaimsStore struct {
	contentToClaims ContentToClaimsMapper
	claimsStore     types.ContentClaimsStore
	claimsAddr      ma.Multiaddr
}

// ContentToClaimsMapper maps content hashes to claim cids
type ContentToClaimsMapper interface {
	GetClaims(ctx context.Context, contentHash multihash.Multihash) (claimsCids []cid.Cid, err error)
}

func NewLegacyClaimsStore(contentToClaimsMapper ContentToClaimsMapper, claimStore types.ContentClaimsStore, claimsUrl string) (LegacyClaimsStore, error) {
	legacyClaimsUrl, err := url.Parse(claimsUrl)
	if err != nil {
		return LegacyClaimsStore{}, err
	}
	claimsAddr, err := maurl.FromURL(legacyClaimsUrl)
	if err != nil {
		return LegacyClaimsStore{}, err
	}

	return LegacyClaimsStore{
		contentToClaims: contentToClaimsMapper,
		claimsStore:     claimStore,
		claimsAddr:      claimsAddr,
	}, nil
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

		pr, err := ls.synthetizeProviderResult(claim)
		if err != nil {
			return nil, err
		}

		results = append(results, pr)
	}

	return results, nil
}

// synthetizeProviderResult synthetizes a provider result, including metadata, from a given claim
func (ls LegacyClaimsStore) synthetizeProviderResult(claim delegation.Delegation) (model.ProviderResult, error) {
	expiration := int64(0)
	if claim.Expiration() != nil {
		expiration = int64(*claim.Expiration())
	}

	claimCid := claim.Link().(cidlink.Link).Cid

	if len(claim.Capabilities()) != 1 {
		return model.ProviderResult{}, fmt.Errorf("claim %s has an unexpected number of capabilities (%d)", claimCid, len(claim.Capabilities()))
	}

	cap := claim.Capabilities()[0]
	switch cap.Can() {
	case assert.LocationAbility:
		caveats, err := assert.LocationCaveatsReader.Read(cap.Nb())
		if err != nil {
			return model.ProviderResult{}, err
		}
		return ls.synthetizeLocationProviderResult(caveats, claimCid, expiration)

	case assert.IndexAbility:
		caveats, err := assert.IndexCaveatsReader.Read(cap.Nb())
		if err != nil {
			return model.ProviderResult{}, err
		}
		return ls.synthetizeIndexProviderResult(caveats, claimCid, expiration)

	case assert.EqualsAbility:
		caveats, err := assert.EqualsCaveatsReader.Read(cap.Nb())
		if err != nil {
			return model.ProviderResult{}, err
		}
		return ls.synthetizeEqualsProviderResult(caveats, claimCid, expiration)

	default:
		return model.ProviderResult{}, fmt.Errorf("unknown claim type: %T", claim.Capabilities()[0])
	}
}

func (ls LegacyClaimsStore) synthetizeLocationProviderResult(caveats assert.LocationCaveats, claimCid cid.Cid, expiration int64) (model.ProviderResult, error) {
	var encodedCtxID types.EncodedContextID
	if caveats.Space != did.Undef {
		spaceDid := caveats.Space
		contextID, err := types.ContextID{
			Hash:  caveats.Content.Hash(),
			Space: &spaceDid,
		}.ToEncoded()
		if err != nil {
			return model.ProviderResult{}, err
		}

		encodedCtxID = contextID
	} else {
		contextID, err := types.ContextID{
			Hash: caveats.Content.Hash(),
		}.ToEncoded()
		if err != nil {
			return model.ProviderResult{}, err
		}

		encodedCtxID = contextID
	}

	contentCid := cid.NewCidV1(cid.Raw, caveats.Content.Hash())
	var rng *metadata.Range
	if caveats.Range != nil {
		rng = &metadata.Range{
			Offset: caveats.Range.Offset,
			Length: caveats.Range.Length,
		}
	}
	meta := metadata.LocationCommitmentMetadata{
		Shard:      &contentCid,
		Range:      rng,
		Expiration: expiration,
		Claim:      claimCid,
	}
	metaBytes, err := meta.MarshalBinary()
	if err != nil {
		return model.ProviderResult{}, err
	}

	providerAddrs := make([]ma.Multiaddr, 0, len(caveats.Location)+1)
	for _, l := range caveats.Location {
		// generalize the location URL by replacing actual hashes with the placeholder.
		// That will allow the correct URL to be reconstructed for fetching
		l.Path = strings.ReplaceAll(l.Path, digestutil.Format(caveats.Content.Hash()), "{blob}")
		ma, err := maurl.FromURL(&l)
		if err != nil {
			return model.ProviderResult{}, err
		}

		providerAddrs = append(providerAddrs, ma)
	}

	// the URL to fetch claims from is also needed
	providerAddrs = append(providerAddrs, ls.claimsAddr)

	providerAddrInfo := &peer.AddrInfo{
		ID:    "",
		Addrs: providerAddrs,
	}

	return model.ProviderResult{
		ContextID: encodedCtxID,
		Metadata:  metaBytes,
		Provider:  providerAddrInfo,
	}, nil
}

func (ls LegacyClaimsStore) synthetizeIndexProviderResult(caveats assert.IndexCaveats, claimCid cid.Cid, expiration int64) (model.ProviderResult, error) {
	indexCid := caveats.Index.(cidlink.Link).Cid
	contextID := []byte(caveats.Index.Binary())

	meta := metadata.IndexClaimMetadata{
		Index:      indexCid,
		Expiration: int64(expiration),
		Claim:      claimCid,
	}
	metaBytes, err := meta.MarshalBinary()
	if err != nil {
		return model.ProviderResult{}, err
	}

	// the index claim is fetchable from the legacy claims store
	providerAddrInfo := &peer.AddrInfo{
		ID:    "",
		Addrs: []ma.Multiaddr{ls.claimsAddr},
	}

	return model.ProviderResult{
		ContextID: contextID,
		Metadata:  metaBytes,
		Provider:  providerAddrInfo,
	}, nil
}

func (ls LegacyClaimsStore) synthetizeEqualsProviderResult(caveats assert.EqualsCaveats, claimCid cid.Cid, expiration int64) (model.ProviderResult, error) {
	equalsCid := caveats.Equals.(cidlink.Link).Cid
	contextID := []byte(caveats.Equals.Binary())

	meta := metadata.EqualsClaimMetadata{
		Equals:     equalsCid,
		Expiration: int64(expiration),
		Claim:      claimCid,
	}
	metaBytes, err := meta.MarshalBinary()
	if err != nil {
		return model.ProviderResult{}, err
	}

	// the equals claim is fetchable from the legacy claims store
	providerAddrInfo := &peer.AddrInfo{
		ID:    "",
		Addrs: []ma.Multiaddr{ls.claimsAddr},
	}

	return model.ProviderResult{
		ContextID: contextID,
		Metadata:  metaBytes,
		Provider:  providerAddrInfo,
	}, nil
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
