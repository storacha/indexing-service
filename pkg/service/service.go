package service

import (
	"context"
	"errors"
	"net/url"

	"github.com/ipfs/go-cid"
	"github.com/ipni/go-libipni/find/model"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multicodec"
	"github.com/multiformats/go-multihash"
	"github.com/storacha-network/go-ucanto/core/delegation"
	"github.com/storacha-network/go-ucanto/did"
	"github.com/storacha-network/indexing-service/pkg/blobindex"
	"github.com/storacha-network/indexing-service/pkg/internal/parallelwalk"
	"github.com/storacha-network/indexing-service/pkg/metadata"
	"github.com/storacha-network/indexing-service/pkg/service/providerindex"
	"github.com/storacha-network/indexing-service/pkg/types"
)

const defaultConcurrency = 5

type Match struct {
	Subject []did.DID
}

type Query struct {
	Hashes []multihash.Multihash
	Match  Match
}

type QueryResult struct {
	Claims  []delegation.Delegation
	Indexes []blobindex.ShardedDagIndexView
}

// ProviderIndex is a read/write interface to a local cache of providers that falls back to IPNI
type ProviderIndex interface {
	// Find should do the following
	//  1. Read from the IPNI Storage cache to get a list of providers
	//     a. If there is no record in cache, query IPNI, filter out any non-content claims metadata, and store
	//     the resulting records in the cache
	//     b. the are no records in the cache or IPNI, it can attempt to read from legacy systems -- Dynamo tables & content claims storage, synthetically constructing provider results
	//  2. With returned provider results, filter additionally for claim type. If space dids are set, calculate an encodedcontextid's by hashing space DID and Hash, and filter for a matching context id
	//     Future TODO: kick off a conversion task to update the recrds
	Find(context.Context, providerindex.QueryKey) ([]model.ProviderResult, error)
	// Publish should do the following:
	// 1. Write the entries to the cache with no expiration until publishing is complete
	// 2. Generate an advertisement for the advertised hashes and publish/announce it
	Publish(context.Context, []multihash.Multihash, model.ProviderResult)
}

// ClaimLookup is used to get full claims from a claim cid
type ClaimLookup interface {
	// LookupClaim should:
	// 1. attempt to read the claim from the cache from the encoded contextID
	// 2. if not found, attempt to fetch the claim from the provided URL. Store the result in cache
	// 3. return the claim
	LookupClaim(ctx context.Context, claimCid cid.Cid, fetchURL url.URL) (delegation.Delegation, error)
}

// BlobIndexLookup is a read through cache for fetching blob indexes
type BlobIndexLookup interface {
	// Find should:
	// 1. attempt to read the sharded dag index from the cache from the encoded contextID
	// 2. if not found, attempt to fetch the index from the provided URL. Store the result in cache
	// 3. return the index
	// 4. asyncronously, add records to the IPNICache from the parsed blob index so that we can avoid future queries to IPNI for
	// other multihashes in the index
	Find(ctx context.Context, contextID types.EncodedContextID, fetchURL url.URL, rng *metadata.Range) (blobindex.ShardedDagIndexView, error)
}

type IndexingService struct {
	blobIndexLookup BlobIndexLookup
	claimLookup     ClaimLookup
	providerIndex   ProviderIndex
}

type job struct {
	mh           multihash.Multihash
	isIndex      bool
	targetClaims []multicodec.Code
}

var allClaims = []multicodec.Code{metadata.EqualsClaimID, metadata.IndexClaimID, metadata.LocationCommitmentID}
var justLocation = []multicodec.Code{metadata.LocationCommitmentID}
var equalsOrLocation = []multicodec.Code{metadata.IndexClaimID, metadata.LocationCommitmentID}

// Query returns back relevant content claims for the given query using the following steps
// 1. Query the IPNIIndex for all matching records
// 2. For any index records, query the IPNIIndex for any location claims for that index cid
// 3. For any index claims, query the IPNIIndex for location claims for the index cid
// 4. Query the BlobIndexLookup to get the full ShardedDagIndex for any index claims
// 5. Query IPNIIndex for any location claims for any shards that contain the multihash based on the ShardedDagIndex
// 6. Read the requisite claims from the ClaimLookup
// 7. Return all discovered claims and sharded dag indexes
func (is *IndexingService) Query(ctx context.Context, q Query) (QueryResult, error) {
	initialJobs := make([]job, 0, len(q.Hashes))
	for _, mh := range q.Hashes {
		initialJobs = append(initialJobs, job{mh, false, allClaims})
	}
	return parallelwalk.ParallelWalk(ctx, initialJobs, QueryResult{}, func(mhCtx context.Context, j job, spawn func(job) error, stateModifier func(func(QueryResult) QueryResult)) error {
		results, err := is.providerIndex.Find(mhCtx, providerindex.QueryKey{
			Hash:         j.mh,
			Spaces:       q.Match.Subject,
			TargetClaims: j.targetClaims,
		})
		if err != nil {
			return err
		}
		for _, result := range results {
			md := metadata.MetadataContext.New()
			err = md.UnmarshalBinary(result.Metadata)
			if err != nil {
				return err
			}
			for _, code := range md.Protocols() {
				protocol := md.Get(code)
				if hasClaimCid, ok := protocol.(metadata.HasClaimCid); ok {
					claimCid := hasClaimCid.GetClaimCid()
					url := is.fetchClaimUrl(*result.Provider, claimCid)
					claim, err := is.claimLookup.LookupClaim(mhCtx, claimCid, url)
					if err != nil {
						return err
					}
					stateModifier(func(qr QueryResult) QueryResult {
						qr.Claims = append(qr.Claims, claim)
						return qr
					})
				}
				switch typedProtocol := protocol.(type) {
				case *metadata.EqualsClaimMetadata:
					if string(typedProtocol.Equals.Hash()) != string(j.mh) {
						// lookup was the content hash, queue the equals hash
						if err := spawn(job{typedProtocol.Equals.Hash(), false, justLocation}); err != nil {
							return err
						}
					} else {
						// lookup was the equals hash, queue the content hash
						if err := spawn(job{multihash.Multihash(result.ContextID), false, justLocation}); err != nil {
							return err
						}
					}
				case *metadata.IndexClaimMetadata:
					if err := spawn(job{typedProtocol.Index.Hash(), true, justLocation}); err != nil {
						return err
					}
				case *metadata.LocationCommitmentMetadata:
					if j.isIndex {
						shard := typedProtocol.Shard
						if shard == nil {
							c := cid.NewCidV1(cid.Raw, j.mh)
							shard = &c
						}
						url := is.fetchRetrievalUrl(*result.Provider, *shard)
						index, err := is.blobIndexLookup.Find(mhCtx, result.ContextID, url, typedProtocol.Range)
						if err != nil {
							return err
						}
						stateModifier(func(qr QueryResult) QueryResult {
							qr.Indexes = append(qr.Indexes, index)
							return qr
						})
						shards := index.Shards().Iterator()
						for shard := range shards {
							if err := spawn(job{shard, false, equalsOrLocation}); err != nil {
								return err
							}
						}
					}
				}
			}
		}
		return nil
	}, defaultConcurrency)
}

func (is *IndexingService) fetchClaimUrl(provider peer.AddrInfo, claimCid cid.Cid) url.URL {
	// Todo figure out how this works
	return url.URL{}
}

func (is *IndexingService) fetchRetrievalUrl(provider peer.AddrInfo, shard cid.Cid) url.URL {
	// Todo figure out how this works
	return url.URL{}
}

// CacheClaim is used to cache a claim without publishing it to IPNI
// this is used cache a location commitment that come from a storage provider on blob/accept, without publishing, since the SP will publish themselves
// (a delegation for a location commitment is already generated on blob/accept)
// ideally however, IPNI would enable UCAN chains for publishing so that we could publish it directly from the storage service
// it doesn't for now, so we let SPs publish themselves them direct cache with us
func (is *IndexingService) CacheClaim(ctx context.Context, claim delegation.Delegation) error {
	return errors.New("not implemented")
}

// I imagine publish claim to work as follows
// For all claims except index, just use the publish API on IPNIIndex
// For index claims, let's assume they fail if a location claim for the index car cid is not already published
// The service should lookup the index cid location claim, and fetch the ShardedDagIndexView, then use the hashes inside
// to assemble all the multihashes in the index advertisement
func (is *IndexingService) PublishClaim(ctx context.Context, claim delegation.Delegation) error {
	return errors.New("not implemented")
}
