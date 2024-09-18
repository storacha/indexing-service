package blobindexlookup

import (
	"net/url"

	"github.com/ipni/go-libipni/find/model"
	"github.com/storacha-network/indexing-service/pkg/blobindex"
	"github.com/storacha-network/indexing-service/pkg/types"
)

// BlobIndexLookup is a read through cache for fetching blob indexes
type BlobIndexLookup interface {
	// Find should:
	// 1. attempt to read the sharded dag index from the cache from the encoded contextID in the provided ProviderResult
	// 2. if not found, attempt to fetch the index from the provided URL. Store the result in cache
	// 3. return the index
	// 4. asyncronously, add records to the IPNICache from the parsed blob index so that we can avoid future queries to IPNI for
	// other multihashes in the index
	Find(indexRecord model.ProviderResult, fetchURL url.URL) (blobindex.ShardedDagIndexView, error)
}

func NewBlobIndex(blobCache types.ShardedDagIndexStore, ipniCache types.IPNIStore) BlobIndexLookup {
	return nil
}
