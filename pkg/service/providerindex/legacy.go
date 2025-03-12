package providerindex

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/storacha/go-libstoracha/capabilities/assert"
	"github.com/storacha/go-libstoracha/metadata"
	"github.com/storacha/indexing-service/pkg/internal/digestutil"
	"github.com/storacha/indexing-service/pkg/internal/link"
	"github.com/storacha/indexing-service/pkg/service/contentclaims"
	"github.com/storacha/indexing-service/pkg/types"
	"golang.org/x/exp/slices"

	"github.com/ipfs/go-cid"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/ipni/go-libipni/find/model"
	"github.com/ipni/go-libipni/maurl"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/multiformats/go-multicodec"
	"github.com/multiformats/go-multihash"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/go-ucanto/did"
)

// PeerID is the peer ID used in synthetized provider results.
var PeerID, _ = peer.Decode("12D3KooWLrikEsjt5wz326bRhCyEThRhJ936o13c5Ej7ttLbkxgp")

var ErrIgnoreFiltered = errors.New("claim type is not in list of target claims")

// LegacyClaimsFinder is a read-only interface to find claims on a legacy system
type LegacyClaimsFinder interface {
	// Find returns a list of claims for a given content hash.
	// Implementations should return an empty slice and no error if no results are found.
	Find(ctx context.Context, contentHash multihash.Multihash, targetClaims []multicodec.Code) ([]model.ProviderResult, error)
}

// LegacyClaimsStore allows finding claims on a legacy store
type LegacyClaimsStore struct {
	mappers     []ContentToClaimsMapper
	claimsStore contentclaims.Finder
	claimsAddr  ma.Multiaddr
}

// ContentToClaimsMapper maps content hashes to claim cids
type ContentToClaimsMapper interface {
	GetClaims(ctx context.Context, contentHash multihash.Multihash) (claimsCids []cid.Cid, err error)
}

// NewLegacyClaimsStore builds a new store able to find claims in legacy services.
//
// It uses a series of mappers to fetch claims from. Mappers will be consulted in order, so their positions in the list
// define their priority, with the first position being the top priority. This is important because the claims returned
// by Find will be the ones coming from the first mapper that returns relevant claims.
func NewLegacyClaimsStore(contentToClaimsMappers []ContentToClaimsMapper, claimStore contentclaims.Finder, claimsUrl string) (LegacyClaimsStore, error) {
	legacyClaimsUrl, err := url.Parse(claimsUrl)
	if err != nil {
		return LegacyClaimsStore{}, err
	}
	claimsAddr, err := maurl.FromURL(legacyClaimsUrl)
	if err != nil {
		return LegacyClaimsStore{}, err
	}

	return LegacyClaimsStore{
		mappers:     contentToClaimsMappers,
		claimsStore: claimStore,
		claimsAddr:  claimsAddr,
	}, nil
}

// Find looks for the corresponding claims for a given content hash in the mapper and then fetches the claims from the
// claims store.
// Find will look for relevant claims (as indicated by targetClaims) in content-to-claims mappers in the order they
// were specified when this LegacyClaimsStore was created (see NewLegacyClaimsStore). As soon as a mapper returns
// relevant claims, these will be returned and no more mappers will be checked.
func (ls LegacyClaimsStore) Find(ctx context.Context, contentHash multihash.Multihash, targetClaims []multicodec.Code) ([]model.ProviderResult, error) {
	for _, mapper := range ls.mappers {
		results, err := ls.findInMapper(ctx, contentHash, targetClaims, mapper)
		if err != nil {
			return nil, err
		}

		if len(results) > 0 {
			return results, nil
		}
	}

	return []model.ProviderResult{}, nil
}

func (ls LegacyClaimsStore) findInMapper(ctx context.Context, contentHash multihash.Multihash, targetClaims []multicodec.Code, mapper ContentToClaimsMapper) ([]model.ProviderResult, error) {
	claimsCids, err := mapper.GetClaims(ctx, contentHash)
	if err != nil {
		if errors.Is(err, types.ErrKeyNotFound) {
			return []model.ProviderResult{}, nil
		}

		return nil, err
	}

	results := []model.ProviderResult{}

	for _, claimCid := range claimsCids {
		claim, err := ls.claimsStore.Find(ctx, cidlink.Link{Cid: claimCid}, &url.URL{})
		if err != nil {
			if errors.Is(err, types.ErrKeyNotFound) {
				continue
			}

			return nil, err
		}

		pr, err := ls.synthetizeProviderResult(claimCid, claim, targetClaims)
		if err != nil {
			if !errors.Is(err, ErrIgnoreFiltered) {
				log.Warnf("error synthetizing provider result for claim %s: %s", claimCid, err)
			}
			continue
		}

		results = append(results, pr)
	}

	return results, nil
}

// synthetizeProviderResult synthetizes a provider result, including metadata, from a given claim
func (ls LegacyClaimsStore) synthetizeProviderResult(claimCid cid.Cid, claim delegation.Delegation, targetClaims []multicodec.Code) (model.ProviderResult, error) {
	expiration := int64(0)
	if claim.Expiration() != nil {
		expiration = int64(*claim.Expiration())
	}

	if len(claim.Capabilities()) != 1 {
		return model.ProviderResult{}, fmt.Errorf("claim %s has an unexpected number of capabilities (%d)", claimCid, len(claim.Capabilities()))
	}

	cap := claim.Capabilities()[0]
	switch cap.Can() {
	case assert.LocationAbility:
		if !slices.Contains(targetClaims, metadata.LocationCommitmentID) {
			return model.ProviderResult{}, ErrIgnoreFiltered
		}
		caveats, err := assert.LocationCaveatsReader.Read(cap.Nb())
		if err != nil {
			return model.ProviderResult{}, err
		}
		return ls.synthetizeLocationProviderResult(caveats, claimCid, expiration)

	case assert.IndexAbility:
		if !slices.Contains(targetClaims, metadata.IndexClaimID) {
			return model.ProviderResult{}, ErrIgnoreFiltered
		}
		caveats, err := assert.IndexCaveatsReader.Read(cap.Nb())
		if err != nil {
			return model.ProviderResult{}, err
		}
		return ls.synthetizeIndexProviderResult(caveats, claimCid, expiration)

	case assert.EqualsAbility:
		if !slices.Contains(targetClaims, metadata.EqualsClaimID) {
			return model.ProviderResult{}, ErrIgnoreFiltered
		}
		caveats, err := assert.EqualsCaveatsReader.Read(cap.Nb())
		if err != nil {
			return model.ProviderResult{}, err
		}
		return ls.synthetizeEqualsProviderResult(caveats, claimCid, expiration)

	default:
		return model.ProviderResult{}, fmt.Errorf("unsupported capability: %s", cap.Can())
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
		ID:    PeerID,
		Addrs: providerAddrs,
	}

	return model.ProviderResult{
		ContextID: encodedCtxID,
		Metadata:  metaBytes,
		Provider:  providerAddrInfo,
	}, nil
}

func (ls LegacyClaimsStore) synthetizeIndexProviderResult(caveats assert.IndexCaveats, claimCid cid.Cid, expiration int64) (model.ProviderResult, error) {
	indexCid := link.ToCID(caveats.Index)
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
		ID:    PeerID,
		Addrs: []ma.Multiaddr{ls.claimsAddr},
	}

	return model.ProviderResult{
		ContextID: contextID,
		Metadata:  metaBytes,
		Provider:  providerAddrInfo,
	}, nil
}

func (ls LegacyClaimsStore) synthetizeEqualsProviderResult(caveats assert.EqualsCaveats, claimCid cid.Cid, expiration int64) (model.ProviderResult, error) {
	equalsCid := link.ToCID(caveats.Equals)
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
		ID:    PeerID,
		Addrs: []ma.Multiaddr{ls.claimsAddr},
	}

	return model.ProviderResult{
		ContextID: contextID,
		Metadata:  metaBytes,
		Provider:  providerAddrInfo,
	}, nil
}

// NoResultsLegacyClaimsFinder is a LegacyClaimsFinder that returns no results. It can be used when accessing claims
// in a legacy system is not required
type NoResultsLegacyClaimsFinder struct{}

func NewNoResultsLegacyClaimsFinder() NoResultsLegacyClaimsFinder {
	return NoResultsLegacyClaimsFinder{}
}

// Find always returns no results
func (f NoResultsLegacyClaimsFinder) Find(ctx context.Context, contentHash multihash.Multihash, targetClaims []multicodec.Code) ([]model.ProviderResult, error) {
	return []model.ProviderResult{}, nil
}
