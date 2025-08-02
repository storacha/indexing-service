package blobindexlookup

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/ipni/go-libipni/find/model"
	"github.com/storacha/go-libstoracha/blobindex"
	"github.com/storacha/go-libstoracha/metadata"
	"github.com/storacha/indexing-service/pkg/service/providercacher"
	"github.com/storacha/indexing-service/pkg/types"
)

type cachingLookup struct {
	blobIndexLookup    BlobIndexLookup
	shardDagIndexCache types.ShardedDagIndexStore
	cachingQueue       providercacher.CachingQueueQueuer
}

var _ BlobIndexLookup = (*cachingLookup)(nil)

// WithCache returns a blobIndexLookup that attempts to read blobs from the cache, and also caches providers asociated with index cids
func WithCache(blobIndexLookup BlobIndexLookup, shardedDagIndexCache types.ShardedDagIndexStore, cachingQueue providercacher.CachingQueueQueuer) BlobIndexLookup {
	return &cachingLookup{
		blobIndexLookup:    blobIndexLookup,
		shardDagIndexCache: shardedDagIndexCache,
		cachingQueue:       cachingQueue,
	}
}

func (b *cachingLookup) Find(ctx context.Context, contextID types.EncodedContextID, provider model.ProviderResult, fetchURL *url.URL, rng *metadata.Range) (blobindex.ShardedDagIndexView, error) {
	// attempt to read index from cache and return it if succesful
	index, err := b.shardDagIndexCache.Get(ctx, contextID)
	if err == nil {
		return index, nil
	}

	// if an error occurred other than the index not being in the cache, return it
	if !errors.Is(err, types.ErrKeyNotFound) {
		return nil, fmt.Errorf("reading from index cache: %w", err)
	}

	// attempt to fetch the index from the underlying blob index lookup
	index, err = b.blobIndexLookup.Find(ctx, contextID, provider, fetchURL, rng)
	if err != nil {
		return nil, fmt.Errorf("fetching underlying index: %w", err)
	}

	// cache the index for the future
	if err := b.shardDagIndexCache.Set(ctx, contextID, index, true); err != nil {
		return nil, fmt.Errorf("caching fetched index: %w", err)
	}

	// queue a background cache of an provider record for all cids in the index without one
	err = b.cachingQueue.Queue(ctx, providercacher.ProviderCachingJob{
		Provider: provider,
		Index:    index,
	})
	if err != nil {
		return nil, fmt.Errorf("queueing provider caching for index failed: %w", err)
	}

	return index, nil
}
