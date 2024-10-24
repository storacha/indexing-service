package blobindexlookup

import (
	"context"
	"net/url"

	"github.com/ipni/go-libipni/find/model"
	"github.com/storacha/go-metadata"
	"github.com/storacha/indexing-service/pkg/blobindex"
	"github.com/storacha/indexing-service/pkg/types"
)

// BlobIndexLookup is a read through cache for fetching blob indexes
type BlobIndexLookup interface {
	// Find should:
	// 1. attempt to read the sharded dag index from the cache from the encoded contextID
	// 2. if not found, attempt to fetch the index from the provided URL. Store the result in cache
	// 3. return the index
	// 4. asyncronously, add records to the ProviderStore from the parsed blob index so that we can avoid future queries to IPNI for
	// other multihashes in the index
	Find(ctx context.Context, contextID types.EncodedContextID, provider model.ProviderResult, fetchURL url.URL, rng *metadata.Range) (blobindex.ShardedDagIndexView, error)
}
