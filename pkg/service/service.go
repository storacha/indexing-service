package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"sync"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/ipni/go-libipni/find/model"
	"github.com/ipni/go-libipni/maurl"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/multiformats/go-multicodec"
	"github.com/multiformats/go-multihash"
	"github.com/storacha/go-capabilities/pkg/assert"
	"github.com/storacha/go-metadata"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/go-ucanto/principal/ed25519/verifier"
	"github.com/storacha/go-ucanto/validator"
	"github.com/storacha/indexing-service/pkg/blobindex"
	"github.com/storacha/indexing-service/pkg/bytemap"
	"github.com/storacha/indexing-service/pkg/internal/digestutil"
	"github.com/storacha/indexing-service/pkg/internal/jobwalker"
	"github.com/storacha/indexing-service/pkg/internal/jobwalker/parallelwalk"
	"github.com/storacha/indexing-service/pkg/internal/jobwalker/singlewalk"
	"github.com/storacha/indexing-service/pkg/internal/link"
	"github.com/storacha/indexing-service/pkg/service/blobindexlookup"
	"github.com/storacha/indexing-service/pkg/service/contentclaims"
	"github.com/storacha/indexing-service/pkg/service/providerindex"
	"github.com/storacha/indexing-service/pkg/service/queryresult"
	"github.com/storacha/indexing-service/pkg/types"
)

const (
	ClaimUrlPlaceholder = "{claim}"
	blobUrlPlaceholder  = "{blob}"
)

var ErrUnrecognizedClaim = errors.New("unrecognized claim type")

// IndexingService implements read/write logic for indexing data with IPNI, content claims, sharded dag indexes, and a cache layer
type IndexingService struct {
	blobIndexLookup blobindexlookup.BlobIndexLookup
	claims          contentclaims.Service
	providerIndex   providerindex.ProviderIndex
	// provider is the peer info for this service, used when publishing claims.
	provider  peer.AddrInfo
	jobWalker jobwalker.JobWalker[job, queryState]
}

var _ types.Service = (*IndexingService)(nil)

type job struct {
	mh                  multihash.Multihash
	indexForMh          *multihash.Multihash
	indexProviderRecord *model.ProviderResult
	queryType           types.QueryType
}

type jobKey string

func (j job) key() jobKey {
	k := jobKey(j.mh) + jobKey(j.queryType.String())
	if j.indexForMh != nil {
		k += jobKey(*j.indexForMh)
	}
	return k
}

var targetClaims = map[types.QueryType][]multicodec.Code{
	types.QueryTypeStandard:        {metadata.EqualsClaimID, metadata.IndexClaimID, metadata.LocationCommitmentID},
	types.QueryTypeLocation:        {metadata.LocationCommitmentID},
	types.QueryTypeIndexOrLocation: {metadata.IndexClaimID, metadata.LocationCommitmentID},
}

type queryResult struct {
	Claims  map[cid.Cid]delegation.Delegation
	Indexes bytemap.ByteMap[types.EncodedContextID, blobindex.ShardedDagIndexView]
}

type queryState struct {
	q      *types.Query
	qr     *queryResult
	visits map[jobKey]struct{}
}

func (is *IndexingService) jobHandler(mhCtx context.Context, j job, spawn func(job) error, state jobwalker.WrappedState[queryState]) error {

	// check if node has already been visited and ignore if that is the case
	if !state.CmpSwap(func(qs queryState) bool {
		_, ok := qs.visits[j.key()]
		return !ok
	}, func(qs queryState) queryState {
		qs.visits[j.key()] = struct{}{}
		return qs
	}) {
		return nil
	}

	// find provider records related to this multihash
	results, err := is.providerIndex.Find(mhCtx, providerindex.QueryKey{
		Hash:         j.mh,
		Spaces:       state.Access().q.Match.Subject,
		TargetClaims: targetClaims[j.queryType],
	})
	if err != nil {
		return err
	}
	for _, result := range results {
		// unmarshall metadata for this provider
		md := metadata.MetadataContext.New()
		err = md.UnmarshalBinary(result.Metadata)
		if err != nil {
			return err
		}
		// the provider may list one or more protocols for this CID
		// in our case, the protocols are just differnt types of content claims
		for _, code := range md.Protocols() {
			protocol := md.Get(code)
			// make sure this is some kind of claim protocol, ignore if not
			hasClaimCid, ok := protocol.(metadata.HasClaim)
			if !ok {
				continue
			}
			// fetch (from cache or url) the actual content claim
			claimCid := hasClaimCid.GetClaim()
			url, err := fetchClaimURL(*result.Provider, claimCid)
			if err != nil {
				return err
			}
			claim, err := is.claims.Find(mhCtx, cidlink.Link{Cid: claimCid}, url)
			if err != nil {
				return err
			}
			// add the fetched claim to the results, if we don't already have it
			state.CmpSwap(
				func(qs queryState) bool {
					_, ok := qs.qr.Claims[claimCid]
					return !ok
				},
				func(qs queryState) queryState {
					qs.qr.Claims[claimCid] = claim
					return qs
				})

			// handle each type of protocol
			switch typedProtocol := protocol.(type) {
			case *metadata.EqualsClaimMetadata:
				// for an equals claim, it's published on both the content and equals multihashes
				// we follow with a query for location claim on the OTHER side of the multihash
				if string(typedProtocol.Equals.Hash()) != string(j.mh) {
					// lookup was the content hash, queue the equals hash
					if err := spawn(job{typedProtocol.Equals.Hash(), nil, nil, types.QueryTypeLocation}); err != nil {
						return err
					}
				} else {
					// lookup was the equals hash, queue the content hash
					if err := spawn(job{multihash.Multihash(result.ContextID), nil, nil, types.QueryTypeLocation}); err != nil {
						return err
					}
				}
			case *metadata.IndexClaimMetadata:
				// for an index claim, we follow by looking for a location claim for the index, and fetching the index
				mh := j.mh
				if err := spawn(job{typedProtocol.Index.Hash(), &mh, &result, types.QueryTypeIndexOrLocation}); err != nil {
					return err
				}
			case *metadata.LocationCommitmentMetadata:
				// for a location claim, we just store it, unless its for an index CID, in which case get the full idnex
				if j.indexForMh != nil {
					// fetch (from URL or cache) the full index
					shard := typedProtocol.Shard
					if shard == nil {
						c := cid.NewCidV1(cid.Raw, j.mh)
						shard = &c
					}
					url, err := fetchRetrievalURL(*result.Provider, *shard)
					if err != nil {
						return err
					}
					index, err := is.blobIndexLookup.Find(mhCtx, result.ContextID, *j.indexProviderRecord, url, typedProtocol.Range)
					if err != nil {
						return err
					}
					// Add the index to the query results, if we don't already have it
					state.CmpSwap(
						func(qs queryState) bool {
							return !qs.qr.Indexes.Has(result.ContextID)
						},
						func(qs queryState) queryState {
							qs.qr.Indexes.Set(result.ContextID, index)
							return qs
						})

					// add location queries for all shards containing the original CID we're seeing an index for
					shards := index.Shards().Iterator()
					for shard, index := range shards {
						if index.Has(*j.indexForMh) {
							if err := spawn(job{shard, nil, nil, types.QueryTypeIndexOrLocation}); err != nil {
								return err
							}
						}
					}
				}
			}
		}
	}
	return nil
}

// Query returns back relevant content claims for the given query using the following steps
// 1. Query the ProviderIndex for all matching records
// 2. For any index claims, query the ProviderIndex for location claims for the index cid
// 3. Query the BlobIndexLookup to get the full ShardedDagIndex for any index claims
// 4. Query ProviderIndex for any location claims for any shards that contain the multihash based on the ShardedDagIndex
// 5. Read the requisite claims from the ClaimLookup
// 6. Return all discovered claims and sharded dag indexes
func (is *IndexingService) Query(ctx context.Context, q types.Query) (types.QueryResult, error) {
	initialJobs := make([]job, 0, len(q.Hashes))
	for _, mh := range q.Hashes {
		initialJobs = append(initialJobs, job{mh, nil, nil, q.Type})
	}
	qs, err := is.jobWalker(ctx, initialJobs, queryState{
		q: &q,
		qr: &queryResult{
			Claims:  make(map[cid.Cid]delegation.Delegation),
			Indexes: bytemap.NewByteMap[types.EncodedContextID, blobindex.ShardedDagIndexView](-1),
		},
		visits: map[jobKey]struct{}{},
	}, is.jobHandler)
	if err != nil {
		return nil, err
	}
	return queryresult.Build(qs.qr.Claims, qs.qr.Indexes)
}

func urlForResource(provider peer.AddrInfo, resourcePlaceholder string, resourceID string) (*url.URL, error) {
	for _, addr := range provider.Addrs {
		// first, attempt to convert the addr to a url scheme
		url, err := maurl.ToURL(addr)
		// if it can't be converted, skip
		if err != nil {
			continue
		}
		// must be an http url
		if !(url.Scheme == "http" || url.Scheme == "https") {
			continue
		}
		// we must have a place to place the resourceId in the path
		if !strings.Contains(url.Path, resourcePlaceholder) {
			continue
		}
		// ok we have a matching URL, return with all resource placeholders replaced with the id
		url.Path = strings.ReplaceAll(url.Path, resourcePlaceholder, resourceID)
		return url, nil
	}
	return nil, fmt.Errorf("no %s endpoint found", resourcePlaceholder)
}

func fetchClaimURL(provider peer.AddrInfo, claimCid cid.Cid) (*url.URL, error) {
	return urlForResource(provider, ClaimUrlPlaceholder, claimCid.String())
}

func fetchRetrievalURL(provider peer.AddrInfo, shard cid.Cid) (*url.URL, error) {
	return urlForResource(provider, "{blob}", digestutil.Format(shard.Hash()))
}

func (is *IndexingService) Get(ctx context.Context, claim ipld.Link) (delegation.Delegation, error) {
	return is.claims.Get(ctx, claim)
}

// Cache is used to cache a claim without publishing it to IPNI
// this is used cache a location commitment that come from a storage provider on blob/accept, without publishing, since the SP will publish themselves
// (a delegation for a location commitment is already generated on blob/accept)
// ideally however, IPNI would enable UCAN chains for publishing so that we could publish it directly from the storage service
// it doesn't for now, so we let SPs publish themselves them direct cache with us
func (is *IndexingService) Cache(ctx context.Context, provider peer.AddrInfo, claim delegation.Delegation) error {
	return Cache(ctx, is.blobIndexLookup, is.claims, is.providerIndex, provider, claim)
}

// Publish caches and publishes a content claim
// I imagine publish claim to work as follows
// For all claims except index, just use the publish API on ProviderIndex
// For index claims, let's assume they fail if a location claim for the index car cid is not already published
// The service should lookup the index cid location claim, and fetch the ShardedDagIndexView, then use the hashes inside
// to assemble all the multihashes in the index advertisement
func (is *IndexingService) Publish(ctx context.Context, claim delegation.Delegation) error {
	return Publish(ctx, is.blobIndexLookup, is.claims, is.providerIndex, is.provider, claim)
}

// Option configures an IndexingService
type Option func(is *IndexingService)

// WithConcurrency causes the indexing service to process find queries parallel, with the given concurrency
func WithConcurrency(concurrency int) Option {
	return func(is *IndexingService) {
		is.jobWalker = parallelwalk.NewParallelWalk[job, queryState](concurrency)
	}
}

// NewIndexingService returns a new indexing service
func NewIndexingService(blobIndexLookup blobindexlookup.BlobIndexLookup, claims contentclaims.Service, publicAddrInfo peer.AddrInfo, providerIndex providerindex.ProviderIndex, options ...Option) *IndexingService {
	provider := peer.AddrInfo{ID: publicAddrInfo.ID}
	for _, addr := range publicAddrInfo.Addrs {
		claimSuffix, _ := multiaddr.NewMultiaddr("/http-path/" + url.PathEscape("claim/"+ClaimUrlPlaceholder))
		provider.Addrs = append(provider.Addrs, multiaddr.Join(addr, claimSuffix))
	}
	is := &IndexingService{
		blobIndexLookup: blobIndexLookup,
		claims:          claims,
		provider:        provider,
		providerIndex:   providerIndex,
		jobWalker:       singlewalk.SingleWalker[job, queryState],
	}
	for _, option := range options {
		option(is)
	}
	return is
}

func Cache(ctx context.Context, blobIndex blobindexlookup.BlobIndexLookup, claims contentclaims.Service, provIndex providerindex.ProviderIndex, provider peer.AddrInfo, claim delegation.Delegation) error {
	caps := claim.Capabilities()
	if len(caps) == 0 {
		return fmt.Errorf("missing capabilities in claim: %s", claim.Link())
	}

	switch caps[0].Can() {
	case assert.LocationAbility:
		return cacheLocationCommitment(ctx, claims, provIndex, provider, claim)
	default:
		return ErrUnrecognizedClaim
	}
}

func cacheLocationCommitment(ctx context.Context, claims contentclaims.Service, provIndex providerindex.ProviderIndex, provider peer.AddrInfo, claim delegation.Delegation) error {
	capability := claim.Capabilities()[0]
	if capability.Can() != assert.LocationAbility {
		return fmt.Errorf("unsupported claim: %s", capability.Can())
	}

	nb, rerr := assert.LocationCaveatsReader.Read(capability.Nb())
	if rerr != nil {
		return fmt.Errorf("reading index claim data: %w", rerr)
	}

	err := claims.Cache(ctx, claim)
	if err != nil {
		return fmt.Errorf("caching claim with claim service: %w", err)
	}

	digests := []multihash.Multihash{nb.Content.Hash()}
	contextID, err := types.ContextID{Space: &nb.Space, Hash: nb.Content.Hash()}.ToEncoded()
	if err != nil {
		return fmt.Errorf("encoding advertisement context ID: %w", err)
	}

	var exp int
	if claim.Expiration() != nil {
		exp = *claim.Expiration()
	}

	var rng *metadata.Range
	if nb.Range != nil {
		rng = &metadata.Range{Offset: nb.Range.Offset, Length: nb.Range.Length}
	}

	meta := metadata.MetadataContext.New(
		&metadata.LocationCommitmentMetadata{
			Expiration: int64(exp),
			Claim:      link.ToCID(claim.Link()),
			Range:      rng,
		},
	)

	err = provIndex.Cache(ctx, provider, string(contextID), slices.Values(digests), meta)
	if err != nil {
		return fmt.Errorf("caching claim with provider index: %w", err)
	}

	return nil
}

func Publish(ctx context.Context, blobIndex blobindexlookup.BlobIndexLookup, claims contentclaims.Service, provIndex providerindex.ProviderIndex, provider peer.AddrInfo, claim delegation.Delegation) error {
	caps := claim.Capabilities()
	if len(caps) == 0 {
		return fmt.Errorf("missing capabilities in claim: %s", claim.Link())
	}
	switch caps[0].Can() {
	case assert.EqualsAbility:
		return publishEqualsClaim(ctx, claims, provIndex, provider, claim)
	case assert.IndexAbility:
		return publishIndexClaim(ctx, blobIndex, claims, provIndex, provider, claim)
	default:
		return ErrUnrecognizedClaim
	}
}

func publishEqualsClaim(ctx context.Context, claims contentclaims.Service, provIndex providerindex.ProviderIndex, provider peer.AddrInfo, claim delegation.Delegation) error {
	capability := claim.Capabilities()[0]
	nb, rerr := assert.EqualsCaveatsReader.Read(capability.Nb())
	if rerr != nil {
		return fmt.Errorf("reading equals claim data: %w", rerr)
	}

	err := claims.Publish(ctx, claim)
	if err != nil {
		return fmt.Errorf("caching equals claim with claim service: %w", err)
	}

	var exp int
	if claim.Expiration() != nil {
		exp = *claim.Expiration()
	}

	meta := metadata.MetadataContext.New(
		&metadata.EqualsClaimMetadata{
			Equals:     link.ToCID(nb.Equals),
			Expiration: int64(exp),
			Claim:      link.ToCID(claim.Link()),
		},
	)

	var digests []multihash.Multihash
	digests = append(digests, nb.Content.Hash())
	digests = append(digests, nb.Equals.(cidlink.Link).Cid.Hash())
	contextID := nb.Equals.Binary()
	err = provIndex.Publish(ctx, provider, contextID, slices.Values(digests), meta)
	if err != nil {
		return fmt.Errorf("publishing equals claim: %w", err)
	}

	return nil
}

func publishIndexClaim(ctx context.Context, blobIndex blobindexlookup.BlobIndexLookup, claims contentclaims.Service, provIndex providerindex.ProviderIndex, provider peer.AddrInfo, claim delegation.Delegation) error {
	capability := claim.Capabilities()[0]
	nb, rerr := assert.IndexCaveatsReader.Read(capability.Nb())
	if rerr != nil {
		return fmt.Errorf("reading index claim data: %w", rerr)
	}

	err := claims.Publish(ctx, claim)
	if err != nil {
		return fmt.Errorf("caching index claim with claim lookup: %w", err)
	}

	results, err := provIndex.Find(ctx, providerindex.QueryKey{
		Hash:         link.ToCID(nb.Index).Hash(),
		TargetClaims: []multicodec.Code{metadata.LocationCommitmentID},
	})
	if err != nil {
		return fmt.Errorf("finding location commitment: %w", err)
	}
	if len(results) == 0 {
		return fmt.Errorf("no location commitments found for index: %s", nb.Index)
	}

	var idx blobindex.ShardedDagIndex
	var ferr error
	for _, r := range results {
		idx, ferr = fetchBlobIndex(ctx, blobIndex, claims, nb.Index, r)
		if ferr != nil {
			continue
		}
		break
	}
	if ferr != nil {
		return fmt.Errorf("fetching blob index: %w", ferr)
	}

	var exp int
	if claim.Expiration() != nil {
		exp = *claim.Expiration()
	}

	meta := metadata.MetadataContext.New(
		&metadata.IndexClaimMetadata{
			Index:      link.ToCID(nb.Index),
			Expiration: int64(exp),
			Claim:      link.ToCID(claim.Link()),
		},
	)

	digests := bytemap.NewByteMap[multihash.Multihash, struct{}](-1)
	for _, slices := range idx.Shards().Iterator() {
		for d := range slices.Iterator() {
			digests.Set(d, struct{}{})
		}
	}

	contextID := nb.Index.Binary()
	err = provIndex.Publish(ctx, provider, contextID, digests.Keys(), meta)
	if err != nil {
		return fmt.Errorf("publishing index claim: %w", err)
	}

	return nil
}

func fetchBlobIndex(ctx context.Context, blobIndex blobindexlookup.BlobIndexLookup, claims contentclaims.Service, blob ipld.Link, result model.ProviderResult) (blobindex.ShardedDagIndex, error) {
	meta := metadata.MetadataContext.New()
	err := meta.UnmarshalBinary(result.Metadata)
	if err != nil {
		return nil, fmt.Errorf("decoding location commitment metadata: %w", err)
	}

	protocol := meta.Get(metadata.LocationCommitmentID)
	lcmeta, ok := protocol.(*metadata.LocationCommitmentMetadata)
	if !ok {
		return nil, errors.New("metadata is not expected type")
	}

	if lcmeta.Shard != nil {
		blob = cidlink.Link{Cid: *lcmeta.Shard}
	}

	blobURL, err := fetchRetrievalURL(*result.Provider, link.ToCID(blob))
	if err != nil {
		return nil, fmt.Errorf("building retrieval URL: %w", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)

	var validateErr error
	go func() {
		defer wg.Done()
		claimURL, err := fetchClaimURL(*result.Provider, lcmeta.Claim)
		if err != nil {
			validateErr = fmt.Errorf("building claim URL: %w", err)
			return
		}

		dlg, err := claims.Find(ctx, cidlink.Link{Cid: lcmeta.Claim}, claimURL)
		if err != nil {
			validateErr = err
			return
		}

		_, err = validateLocationCommitment(dlg)
		if err != nil {
			validateErr = err
			return
		}
	}()

	// Note: the ContextID here is of a location commitment provider
	idx, err := blobIndex.Find(ctx, result.ContextID, result, blobURL, lcmeta.Range)
	if err != nil {
		return nil, fmt.Errorf("fetching index: %w", err)
	}

	wg.Wait()
	if validateErr != nil {
		return nil, fmt.Errorf("verifying claim: %w", validateErr)
	}

	return idx, nil
}

// validateLocationCommitment ensures that the delegation is a valid UCAN (signed,
// not expired etc.) and is a location commitment.
func validateLocationCommitment(claim delegation.Delegation) (validator.Authorization[assert.LocationCaveats], error) {
	// We use the delegation issuer as the authority, since this should be a self
	// issued UCAN to assert location.
	// TODO: support verifiers for other key types?
	vfr, err := verifier.Parse(claim.Issuer().DID().String())
	if err != nil {
		return nil, err
	}

	vctx := validator.NewValidationContext(
		vfr,
		assert.Location,
		validator.IsSelfIssued,
		// TODO: plug in revocation service?
		func(auth validator.Authorization[any]) validator.Revoked { return nil },
		validator.ProofUnavailable,     // probably don't want to resolve proofs...
		verifier.Parse,                 // TODO: support verifiers for other key types?
		validator.FailDIDKeyResolution, // probably don't want to resolve DID methods either
	)

	auth, err := validator.Access(claim, vctx)
	if err != nil {
		return nil, err
	}

	return auth, nil
}
