package legacy

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
	logging "github.com/ipfs/go-log/v2"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/ipni/go-libipni/find/model"
	"github.com/ipni/go-libipni/maurl"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/multiformats/go-multicodec"
	"github.com/multiformats/go-multihash"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/go-ucanto/did"
)

// ProviderID is the peer ID used in synthetized provider results.
var ProviderID, _ = peer.Decode("12D3KooWLrikEsjt5wz326bRhCyEThRhJ936o13c5Ej7ttLbkxgp")

var ErrIgnoreFiltered = errors.New("claim type is not in list of target claims")

// ClaimsFinder is a read-only interface to find claims on a legacy system
type ClaimsFinder interface {
	// Find returns a list of claims for a given content hash.
	// Implementations should return an empty slice and no error if no results are found.
	Find(ctx context.Context, contentHash multihash.Multihash, targetClaims []multicodec.Code) ([]model.ProviderResult, error)
}

// ClaimsStore allows finding claims on a legacy store
type ClaimsStore struct {
	mappers     []ContentToClaimsMapper
	claimsStore contentclaims.Finder
	claimsAddr  ma.Multiaddr
	log         logging.EventLogger
}

// ContentToClaimsMapper maps content hashes to claim cids
type ContentToClaimsMapper interface {
	GetClaims(ctx context.Context, contentHash multihash.Multihash) (claimsCids []cid.Cid, err error)
}

type config struct {
	log logging.EventLogger
}

// Option configures the ClaimsStore.
type Option func(conf *config)

// WithLogger configures the service to use the passed logger instead of the
// default logger.
func WithLogger(log logging.EventLogger) Option {
	return func(conf *config) {
		conf.log = log
	}
}

// NewClaimsStore builds a new store able to find claims in legacy services.
//
// It uses a series of mappers to fetch claims from. Mappers will be consulted in order, so their positions in the list
// define their priority, with the first position being the top priority. This is important because the claims returned
// by Find will be the ones coming from the first mapper that returns relevant claims.
func NewClaimsStore(contentToClaimsMappers []ContentToClaimsMapper, claimStore contentclaims.Finder, claimsUrl string, options ...Option) (ClaimsStore, error) {
	conf := config{}
	for _, option := range options {
		option(&conf)
	}
	if conf.log == nil {
		conf.log = logging.Logger("legacy")
	}
	legacyClaimsUrl, err := url.Parse(claimsUrl)
	if err != nil {
		return ClaimsStore{}, err
	}
	claimsAddr, err := maurl.FromURL(legacyClaimsUrl)
	if err != nil {
		return ClaimsStore{}, err
	}

	return ClaimsStore{
		mappers:     contentToClaimsMappers,
		claimsStore: claimStore,
		claimsAddr:  claimsAddr,
		log:         conf.log,
	}, nil
}

// Find looks for the corresponding claims for a given content hash in the mapper and then fetches the claims from the
// claims store.
// Find will look for relevant claims (as indicated by targetClaims) in content-to-claims mappers in the order they
// were specified when this LegacyClaimsStore was created (see NewLegacyClaimsStore). As soon as a mapper returns
// relevant claims, these will be returned and no more mappers will be checked.
func (cs ClaimsStore) Find(ctx context.Context, contentHash multihash.Multihash, targetClaims []multicodec.Code) ([]model.ProviderResult, error) {
	for _, mapper := range cs.mappers {
		results, err := cs.findInMapper(ctx, contentHash, targetClaims, mapper)
		if err != nil {
			return nil, err
		}

		if len(results) > 0 {
			return results, nil
		}
	}

	return []model.ProviderResult{}, nil
}

func (cs ClaimsStore) findInMapper(ctx context.Context, contentHash multihash.Multihash, targetClaims []multicodec.Code, mapper ContentToClaimsMapper) ([]model.ProviderResult, error) {
	claimsCids, err := mapper.GetClaims(ctx, contentHash)
	if err != nil {
		if errors.Is(err, types.ErrKeyNotFound) {
			return []model.ProviderResult{}, nil
		}

		return nil, err
	}

	results := []model.ProviderResult{}

	for _, claimCid := range claimsCids {
		claim, err := cs.claimsStore.Find(ctx, cidlink.Link{Cid: claimCid}, &url.URL{})
		if err != nil {
			if errors.Is(err, types.ErrKeyNotFound) {
				continue
			}

			return nil, err
		}

		pr, err := cs.synthetizeProviderResult(claimCid, claim, targetClaims)
		if err != nil {
			if !errors.Is(err, ErrIgnoreFiltered) {
				cs.log.Warnf("error synthetizing provider result for claim %s: %s", claimCid, err)
			}
			continue
		}

		results = append(results, pr)
	}

	return results, nil
}

// synthetizeProviderResult synthetizes a provider result, including metadata, from a given claim
func (cs ClaimsStore) synthetizeProviderResult(claimCid cid.Cid, claim delegation.Delegation, targetClaims []multicodec.Code) (model.ProviderResult, error) {
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
		return cs.synthetizeLocationProviderResult(caveats, claimCid, expiration)

	case assert.IndexAbility:
		if !slices.Contains(targetClaims, metadata.IndexClaimID) {
			return model.ProviderResult{}, ErrIgnoreFiltered
		}
		caveats, err := assert.IndexCaveatsReader.Read(cap.Nb())
		if err != nil {
			return model.ProviderResult{}, err
		}
		return cs.synthetizeIndexProviderResult(caveats, claimCid, expiration)

	case assert.EqualsAbility:
		if !slices.Contains(targetClaims, metadata.EqualsClaimID) {
			return model.ProviderResult{}, ErrIgnoreFiltered
		}
		caveats, err := assert.EqualsCaveatsReader.Read(cap.Nb())
		if err != nil {
			return model.ProviderResult{}, err
		}
		return cs.synthetizeEqualsProviderResult(caveats, claimCid, expiration)

	default:
		return model.ProviderResult{}, fmt.Errorf("unsupported capability: %s", cap.Can())
	}
}

func (cs ClaimsStore) synthetizeLocationProviderResult(caveats assert.LocationCaveats, claimCid cid.Cid, expiration int64) (model.ProviderResult, error) {
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

	var rng *metadata.Range
	if caveats.Range != nil {
		rng = &metadata.Range{
			Offset: caveats.Range.Offset,
			Length: caveats.Range.Length,
		}
	}
	meta := metadata.LocationCommitmentMetadata{
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
	providerAddrs = append(providerAddrs, cs.claimsAddr)

	providerAddrInfo := &peer.AddrInfo{
		ID:    ProviderID,
		Addrs: providerAddrs,
	}

	return model.ProviderResult{
		ContextID: encodedCtxID,
		Metadata:  metaBytes,
		Provider:  providerAddrInfo,
	}, nil
}

func (cs ClaimsStore) synthetizeIndexProviderResult(caveats assert.IndexCaveats, claimCid cid.Cid, expiration int64) (model.ProviderResult, error) {
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
		ID:    ProviderID,
		Addrs: []ma.Multiaddr{cs.claimsAddr},
	}

	return model.ProviderResult{
		ContextID: contextID,
		Metadata:  metaBytes,
		Provider:  providerAddrInfo,
	}, nil
}

func (cs ClaimsStore) synthetizeEqualsProviderResult(caveats assert.EqualsCaveats, claimCid cid.Cid, expiration int64) (model.ProviderResult, error) {
	equalsCid := link.ToCID(caveats.Equals)
	contextID := caveats.Content.Hash()

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
		ID:    ProviderID,
		Addrs: []ma.Multiaddr{cs.claimsAddr},
	}

	return model.ProviderResult{
		ContextID: contextID,
		Metadata:  metaBytes,
		Provider:  providerAddrInfo,
	}, nil
}

// NoResultsClaimsFinder is a LegacyClaimsFinder that returns no results. It can be used when accessing claims
// in a legacy system is not required
type NoResultsClaimsFinder struct{}

func NewNoResultsClaimsFinder() NoResultsClaimsFinder {
	return NoResultsClaimsFinder{}
}

// Find always returns no results
func (f NoResultsClaimsFinder) Find(ctx context.Context, contentHash multihash.Multihash, targetClaims []multicodec.Code) ([]model.ProviderResult, error) {
	return []model.ProviderResult{}, nil
}
