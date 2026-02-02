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
	logging "github.com/ipfs/go-log/v2"
	"github.com/ipld/go-ipld-prime"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/ipni/go-libipni/find/model"
	"github.com/ipni/go-libipni/maurl"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/multiformats/go-multicodec"
	"github.com/multiformats/go-multihash"
	"github.com/storacha/go-libstoracha/advertisement"
	"github.com/storacha/go-libstoracha/capabilities/assert"
	"github.com/storacha/go-libstoracha/capabilities/space/content"
	"github.com/storacha/go-libstoracha/metadata"
	"github.com/storacha/go-ucanto/core/dag/blockstore"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/go-ucanto/core/invocation"
	"github.com/storacha/go-ucanto/core/iterable"
	"github.com/storacha/go-ucanto/did"
	"github.com/storacha/go-ucanto/principal/ed25519/verifier"
	"github.com/storacha/go-ucanto/ucan"
	"github.com/storacha/go-ucanto/validator"
	"go.opentelemetry.io/otel/attribute"

	"github.com/storacha/go-libstoracha/blobindex"
	"github.com/storacha/go-libstoracha/bytemap"
	"github.com/storacha/go-libstoracha/digestutil"
	"github.com/storacha/indexing-service/pkg/internal/jobwalker"
	"github.com/storacha/indexing-service/pkg/internal/jobwalker/parallelwalk"
	"github.com/storacha/indexing-service/pkg/internal/jobwalker/singlewalk"
	"github.com/storacha/indexing-service/pkg/internal/link"
	"github.com/storacha/indexing-service/pkg/service/blobindexlookup"
	"github.com/storacha/indexing-service/pkg/service/contentclaims"
	"github.com/storacha/indexing-service/pkg/service/providerindex"
	"github.com/storacha/indexing-service/pkg/service/queryresult"
	"github.com/storacha/indexing-service/pkg/telemetry"
	"github.com/storacha/indexing-service/pkg/types"
)

const (
	ClaimUrlPlaceholder   = "{claim}"
	blobUrlPlaceholder    = "{blob}"
	blobCIDUrlPlaceholder = "{blobCID}"
)

var log = logging.Logger("service")

var ErrUnrecognizedClaim = errors.New("unrecognized claim type")

// IndexingService implements read/write logic for indexing data with IPNI, content claims, sharded dag indexes, and a cache layer
type IndexingService struct {
	id              ucan.Signer
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
	types.QueryTypeStandard:           {metadata.EqualsClaimID, metadata.IndexClaimID, metadata.LocationCommitmentID},
	types.QueryTypeStandardCompressed: {metadata.EqualsClaimID, metadata.IndexClaimID, metadata.LocationCommitmentID},
	types.QueryTypeLocation:           {metadata.LocationCommitmentID},
	types.QueryTypeIndexOrLocation:    {metadata.IndexClaimID, metadata.LocationCommitmentID},
	types.QueryTypeEqualsOrLocation:   {metadata.EqualsClaimID, metadata.LocationCommitmentID},
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
	mhCtx, s := telemetry.StartSpan(mhCtx, "IndexingService.jobHandler")
	defer s.End()
	s.SetAttributes(attribute.String("multihash", digestutil.Format(j.mh)))

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
	s.AddEvent("finding relevant ProviderResults")
	results, err := is.providerIndex.Find(mhCtx, providerindex.QueryKey{
		Hash:         j.mh,
		Spaces:       state.Access().q.Match.Subject,
		TargetClaims: targetClaims[j.queryType],
	})
	if err != nil {
		telemetry.Error(s, err, "finding ProviderResults")
		return err
	}

	s.AddEvent(fmt.Sprintf("processing %d results", len(results)))

	var indexFetchSucceeded bool
	var lastIndexFetchErr error

	for _, result := range results {
		// unmarshall metadata for this provider
		md := metadata.MetadataContext.New()
		err = md.UnmarshalBinary(result.Metadata)
		if err != nil {
			telemetry.Error(s, err, "unmarshaling metadata")
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
				telemetry.Error(s, err, "building claim URL")
				return err
			}

			s.AddEvent("fetching claims")
			claim, err := is.claims.Find(mhCtx, cidlink.Link{Cid: claimCid}, url)
			if err != nil {
				telemetry.Error(s, err, "fetching claims")
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
				s.AddEvent("processing equals claim")

				// for an equals claim, it's published on both the content and equals multihashes
				// we follow with a query for location claim on the OTHER side of the multihash
				if string(typedProtocol.Equals.Hash()) != string(j.mh) {
					// lookup was the content hash, queue the equals hash
					if err := spawn(job{typedProtocol.Equals.Hash(), nil, nil, types.QueryTypeLocation}); err != nil {
						telemetry.Error(s, err, "queing job for equals hash")
						return err
					}
				} else {
					// lookup was the equals hash, queue the content hash
					if err := spawn(job{multihash.Multihash(result.ContextID), nil, nil, types.QueryTypeLocation}); err != nil {
						telemetry.Error(s, err, "queuing job for content hash")
						return err
					}
				}
			case *metadata.IndexClaimMetadata:
				s.AddEvent("processing index claim")

				// for an index claim, we follow by looking for a location claim for the index, and fetching the index
				mh := j.mh
				if err := spawn(job{typedProtocol.Index.Hash(), &mh, &result, types.QueryTypeLocation}); err != nil {
					telemetry.Error(s, err, "queuing job for the index's location claim")
					return err
				}
			case *metadata.LocationCommitmentMetadata:
				s.AddEvent("processing location claim")

				// for a location claim, we just store it, unless its for an index CID, in which case get the full index
				if j.indexForMh != nil {
					// Try to fetch the index from this provider result
					// If it fails, we'll continue to the next result instead of failing the entire query
					shard := typedProtocol.Shard
					if shard == nil {
						c := cid.NewCidV1(cid.Raw, j.mh)
						shard = &c
					}
					url, err := fetchRetrievalURL(*result.Provider, *shard)
					if err != nil {
						telemetry.Error(s, err, "fetching index retrieval URL")
						log.Warnw("failed to fetch retrieval URL, will try next provider result if available", "shard", shard, "provider", result.Provider.ID, "err", err)
						lastIndexFetchErr = fmt.Errorf("fetching retrieval URL for index %q from provider %s: %w", shard, result.Provider.ID, err)
						continue // Try next provider result
					}

					s.AddEvent("fetching index")
					var auth *types.RetrievalAuth
					match, err := assert.Location.Match(validator.NewSource(claim.Capabilities()[0], claim))
					if err != nil {
						log.Warnw("failed to match claim to location commitment, will try next provider result if available", "err", err)
						lastIndexFetchErr = fmt.Errorf("failed to match claim to location commitment: %w", err)
						continue
					}
					lcCaveats := match.Value().Nb()
					space := lcCaveats.Space
					dlgs := state.Access().q.Delegations
					// Authorized retrieval requires a space in the location claim, a
					// delegation for the retrieval, and an absolute byte range to extract.
					if space != did.Undef && len(dlgs) > 0 && lcCaveats.Range != nil && lcCaveats.Range.Length != nil {
						var proofs []delegation.Proof
						for _, d := range dlgs {
							for _, c := range d.Capabilities() {
								if c.Can() == content.Retrieve.Can() && c.With() == space.String() {
									proofs = append(proofs, delegation.FromDelegation(d))
								}
							}
						}
						if len(proofs) > 0 {
							cap := content.Retrieve.New(space.String(), content.RetrieveCaveats{
								Blob: content.BlobDigest{Digest: lcCaveats.Content.Hash()},
								Range: content.Range{
									Start: lcCaveats.Range.Offset,
									End:   lcCaveats.Range.Offset + *lcCaveats.Range.Length - 1,
								},
							})
							a := types.NewRetrievalAuth(is.id, claim.Issuer(), cap, proofs)
							auth = &a
						}
					}
					req := types.NewRetrievalRequest(url, typedProtocol.Range, auth)
					index, err := is.blobIndexLookup.Find(mhCtx, result.ContextID, *j.indexProviderRecord, req)
					if err != nil {
						telemetry.Error(s, err, "fetching index blob")
						log.Warnw("failed to fetch index blob, will try next provider result if available", "provider", result.Provider.ID, "err", err)
						lastIndexFetchErr = fmt.Errorf("fetching index blob from provider %s: %w", result.Provider.ID, err)
						continue // Try next provider result
					}

					// Success! Add the index to the query results, if we don't already have it
					indexFetchSucceeded = true
					state.CmpSwap(
						func(qs queryState) bool {
							return !qs.qr.Indexes.Has(result.ContextID)
						},
						func(qs queryState) queryState {
							qs.qr.Indexes.Set(result.ContextID, index)
							return qs
						})

					// add location queries for all shards containing the original CID we're seeing an index for
					s.AddEvent("adding location queries for indexed shards")
					shards := index.Shards().Iterator()
					for shard, index := range shards {
						if index.Has(*j.indexForMh) {
							if err := spawn(job{shard, nil, nil, types.QueryTypeLocation}); err != nil {
								telemetry.Error(s, err, "queuing location job for shard")
								return err
							}
						}
					}
				}
			}
		}
	}

	// If we attempted to fetch an index but all attempts failed, return the last error
	if lastIndexFetchErr != nil && !indexFetchSucceeded {
		return fmt.Errorf("failed to fetch index from all provider results: %w", lastIndexFetchErr)
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
	ctx, s := telemetry.StartSpan(ctx, "IndexingService.Query")
	defer s.End()

	if q.Type == types.QueryTypeStandardCompressed && len(q.Hashes) != 1 {
		return nil, fmt.Errorf("invalid query: expected 1 hash for compressed query, got %d", len(q.Hashes))
	}

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
	if q.Type == types.QueryTypeStandardCompressed {
		return queryresult.BuildCompressed(q.Hashes[0], is.id, qs.qr.Claims, qs.qr.Indexes)
	}
	return queryresult.Build(qs.qr.Claims, qs.qr.Indexes)
}

type replacement struct {
	resourcePlaceholder string
	resourceID          string
}

func urlForResource(provider peer.AddrInfo, replacements []replacement) (*url.URL, error) {
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
		replacedAny := false
		for _, replacement := range replacements {
			resourcePlaceholder, resourceID := replacement.resourcePlaceholder, replacement.resourceID
			// we must have a place to place the resourceId in the path
			if !strings.Contains(url.Path, resourcePlaceholder) {
				continue
			}
			replacedAny = true
			// ok we have a matching URL, return with all resource placeholders replaced with the id
			url.Path = strings.ReplaceAll(url.Path, resourcePlaceholder, resourceID)
		}
		if replacedAny {
			return url, nil
		}
	}
	placeholders := strings.Join(slices.Collect(iterable.Map(func(r replacement) string { return r.resourcePlaceholder }, slices.Values(replacements))), " or ")
	addrs := strings.Join(slices.Collect(iterable.Map(func(a multiaddr.Multiaddr) string { return a.String() }, slices.Values(provider.Addrs))), ", ")
	return nil, fmt.Errorf("no %s endpoint found in %d addresses: %s", placeholders, len(provider.Addrs), addrs)
}

func fetchClaimURL(provider peer.AddrInfo, claimCid cid.Cid) (*url.URL, error) {
	return urlForResource(provider, []replacement{{ClaimUrlPlaceholder, claimCid.String()}})
}

func fetchRetrievalURL(provider peer.AddrInfo, shard cid.Cid) (*url.URL, error) {
	return urlForResource(provider, []replacement{{blobUrlPlaceholder, digestutil.Format(shard.Hash())}, {blobCIDUrlPlaceholder, shard.String()}})
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
	return Publish(ctx, is.id, is.blobIndexLookup, is.claims, is.providerIndex, is.provider, claim)
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
func NewIndexingService(id ucan.Signer, blobIndexLookup blobindexlookup.BlobIndexLookup, claims contentclaims.Service, publicAddrInfo peer.AddrInfo, providerIndex providerindex.ProviderIndex, options ...Option) *IndexingService {
	provider := peer.AddrInfo{ID: publicAddrInfo.ID}
	for _, addr := range publicAddrInfo.Addrs {
		claimSuffix, _ := multiaddr.NewMultiaddr("/http-path/" + url.PathEscape("claim/"+ClaimUrlPlaceholder))
		provider.Addrs = append(provider.Addrs, multiaddr.Join(addr, claimSuffix))
	}
	is := &IndexingService{
		id:              id,
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
	ctx, s := telemetry.StartSpan(ctx, "IndexingService.Cache")
	defer s.End()

	caps := claim.Capabilities()
	if len(caps) == 0 {
		return fmt.Errorf("missing capabilities in claim: %s", claim.Link())
	}

	switch caps[0].Can() {
	case assert.LocationAbility:
		s.SetAttributes(attribute.KeyValue{Key: "claim", Value: attribute.StringValue("assert/location")})
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
	contextID, err := advertisement.EncodeContextID(nb.Space, nb.Content.Hash())
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

	shardCid, err := advertisement.ShardCID(provider, nb)
	if err != nil {
		return fmt.Errorf("failed to extract shard CID for provider: %s locationCommitment %s: %w", provider, capability, err)
	}

	meta := metadata.MetadataContext.New(
		&metadata.LocationCommitmentMetadata{
			Shard:      shardCid,
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

func Publish(ctx context.Context, id ucan.Signer, blobIndex blobindexlookup.BlobIndexLookup, claims contentclaims.Service, provIndex providerindex.ProviderIndex, provider peer.AddrInfo, claim delegation.Delegation) error {
	ctx, s := telemetry.StartSpan(ctx, "IndexingService.Publish")
	defer s.End()

	caps := claim.Capabilities()
	if len(caps) == 0 {
		return fmt.Errorf("missing capabilities in claim: %s", claim.Link())
	}
	switch caps[0].Can() {
	case assert.EqualsAbility:
		s.SetAttributes(attribute.KeyValue{Key: "claim", Value: attribute.StringValue("assert/equals")})
		return publishEqualsClaim(ctx, claims, provIndex, provider, claim)
	case assert.IndexAbility:
		s.SetAttributes(attribute.KeyValue{Key: "claim", Value: attribute.StringValue("assert/index")})
		return publishIndexClaim(ctx, id, blobIndex, claims, provIndex, provider, claim)
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
	contextID := string(nb.Content.Hash())
	err = provIndex.Publish(ctx, provider, contextID, slices.Values(digests), meta)
	if err != nil {
		return fmt.Errorf("publishing equals claim: %w", err)
	}

	return nil
}

func publishIndexClaim(ctx context.Context, id ucan.Signer, blobIndex blobindexlookup.BlobIndexLookup, claims contentclaims.Service, provIndex providerindex.ProviderIndex, provider peer.AddrInfo, claim delegation.Delegation) error {
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
		idx, ferr = fetchBlobIndex(ctx, id, blobIndex, claims, nb.Index, r, claim)
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

func fetchBlobIndex(
	ctx context.Context,
	id ucan.Signer,
	blobIndex blobindexlookup.BlobIndexLookup,
	claims contentclaims.Service,
	blobLink ipld.Link,
	result model.ProviderResult,
	cause invocation.Invocation, // supporting context (typically `assert/index`)
) (blobindex.ShardedDagIndex, error) {
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
		blobLink = cidlink.Link{Cid: *lcmeta.Shard}
	}

	blobURL, err := fetchRetrievalURL(*result.Provider, link.ToCID(blobLink))
	if err != nil {
		return nil, fmt.Errorf("building retrieval URL: %w", err)
	}

	aud, err := peerToPrincipal(result.Provider.ID)
	if err != nil {
		return nil, fmt.Errorf("converting provider peer ID to UCAN principal: %w", err)
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

		_, err = validateLocationCommitment(ctx, dlg)
		if err != nil {
			validateErr = err
			return
		}
	}()

	// Try to extract a space/content/retrieve delegation from the assert/index
	// invocation. If this fails then fallback to trying to request without auth.
	// Note: it'll fail for non-UCAN authorized retrieval nodes (legacy).
	cap, dlg, err := extractContentRetrieveDelegation(cause)
	if err != nil {
		log.Warnw("extracting space/content/retrieve delegation", "err", err)
	}

	byteRange := lcmeta.Range
	var auth *types.RetrievalAuth
	if dlg != nil {
		cap := content.Retrieve.New(cap.With(), cap.Nb())
		prfs := []delegation.Proof{delegation.FromDelegation(dlg)}
		a := types.NewRetrievalAuth(id, aud, cap, prfs)
		offset := cap.Nb().Range.Start
		length := cap.Nb().Range.End - cap.Nb().Range.Start + 1
		byteRange = &metadata.Range{Offset: offset, Length: &length}
		auth = &a
	}

	req := types.NewRetrievalRequest(blobURL, byteRange, auth)
	// Note: the ContextID here is of a location commitment provider
	idx, err := blobIndex.Find(ctx, result.ContextID, result, req)
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
func validateLocationCommitment(ctx context.Context, claim delegation.Delegation) (validator.Authorization[assert.LocationCaveats], error) {
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
		func(ctx context.Context, auth validator.Authorization[any]) validator.Revoked { return nil },
		validator.ProofUnavailable,     // probably don't want to resolve proofs...
		verifier.Parse,                 // TODO: support verifiers for other key types?
		validator.FailDIDKeyResolution, // probably don't want to resolve DID methods either
		validator.NotExpiredNotTooEarly,
	)

	auth, err := validator.Access(ctx, claim, vctx)
	if err != nil {
		return nil, err
	}

	return auth, nil
}

// peerToPrincipal converts a peer ID into a UCAN principal object. Currently
// supports only ed25519 keys.
func peerToPrincipal(peer peer.ID) (ucan.Principal, error) {
	pk, err := peer.ExtractPublicKey()
	if err != nil {
		return nil, fmt.Errorf("extracting public key from peer ID: %w", err)
	}
	pubBytes, err := pk.Raw()
	if err != nil {
		return nil, fmt.Errorf("extracting raw bytes of public key: %w", err)
	}
	v, err := verifier.FromRaw(pubBytes)
	if err != nil {
		return nil, fmt.Errorf("decoding raw ed25519 public key: %w", err)
	}
	return v, nil
}

// extractContentRetrieveDelegation extracts a `space/content/retrieve`
// delegation attached to the passed invocation (typically an `assert/index`).
// The delegation is expected to be linked from facts by a "retrievalAuth" key.
func extractContentRetrieveDelegation(assertion invocation.Invocation) (ucan.Capability[content.RetrieveCaveats], delegation.Delegation, error) {
	var root ipld.Link
	for _, f := range assertion.Facts() {
		authValue, ok := f["retrievalAuth"]
		if !ok {
			continue
		}
		node, ok := authValue.(ipld.Node)
		if !ok {
			break
		}
		l, err := node.AsLink()
		if err != nil {
			log.Warnf("auth value is not an IPLD link")
			break
		}
		root = l
		break
	}
	if root == nil {
		return nil, nil, errors.New("retrieval authorization delegation link not found in facts")
	}
	bs, err := blockstore.NewBlockReader(blockstore.WithBlocksIterator(assertion.Blocks()))
	if err != nil {
		return nil, nil, err
	}
	dlg, err := delegation.NewDelegationView(root, bs)
	if err != nil {
		return nil, nil, fmt.Errorf("creating retrieval authorization delegation: %w", err)
	}
	match, err := content.Retrieve.Match(validator.NewSource(dlg.Capabilities()[0], dlg))
	if err != nil {
		return nil, nil, fmt.Errorf("matching %s delegation: %w", content.RetrieveAbility, err)
	}
	return match.Value(), dlg, nil
}
