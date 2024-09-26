package service

import (
	mh "github.com/multiformats/go-multihash"
	"github.com/storacha-network/go-ucanto/core/delegation"
	"github.com/storacha-network/go-ucanto/did"
	"github.com/storacha-network/indexing-service/pkg/blobindex"
)

type Match struct {
	Subject []did.DID
}

type Query struct {
	Hashes []mh.Multihash
	Match  Match
}

type QueryResult struct {
	Claims  []delegation.Delegation
	Indexes []blobindex.ShardedDagIndexView
}

type IndexingService interface {
	// Query returns back relevant content claims for the given query using the following steps
	// 1. Query the IPNIIndex for all matching records
	// 2. For any index records, query the IPNIIndex for any location claims for that index cid
	// 3. For any index claims, query the IPNIIndex for location claims for the index cid
	// 4. Query the BlobIndexLookup to get the full ShardedDagIndex for any index claims
	// 5. Query IPNIIndex for any location claims for any shards that contain the multihash based on the ShardedDagIndex
	// 6. Read the requisite claims from the ClaimLookup
	// 7. Return all discovered claims and sharded dag indexes
	Query(Query) (QueryResult, error)

	// CacheClaim is used to cache a claim without publishing it to IPNI
	// this is used cache a location commitment that come from a storage provider on blob/accept, without publishing, since the SP will publish themselves
	// (a delegation for a location commitment is already generated on blob/accept)
	// ideally however, IPNI would enable UCAN chains for publishing so that we could publish it directly from the storage service
	// it doesn't for now, so we let SPs publish themselves them direct cache with us
	CacheClaim(delegation.Delegation) error

	// I imagine publish claim to work as follows
	// For all claims except index, just use the publish API on IPNIIndex
	// For index claims, let's assume they fail if a location claim for the index car cid is not already published
	// The service should lookup the index cid location claim, and fetch the ShardedDagIndexView, then use the hashes inside
	// to assemble all the multihashes in the index advertisement
	PublishClaim(delegation.Delegation) error
}
