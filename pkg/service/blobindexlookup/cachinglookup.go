package blobindexlookup

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/ipni/go-libipni/find/model"
	"github.com/storacha/indexing-service/pkg/blobindex"
	"github.com/storacha/indexing-service/pkg/metadata"
	"github.com/storacha/indexing-service/pkg/types"
)

type ProviderCacher interface {
	CacheProviderForIndex(ctx context.Context, provider model.ProviderResult, index blobindex.ShardedDagIndexView) error
}

type cachingLookup struct {
	blobIndexLookup    BlobIndexLookup
	shardDagIndexCache types.ShardedDagIndexStore
	providerCacher     ProviderCacher
}

func WithCache(blobIndexLookup BlobIndexLookup, shardedDagIndexCache types.ShardedDagIndexStore, providerCacher ProviderCacher) BlobIndexLookup {
	return &cachingLookup{
		blobIndexLookup:    blobIndexLookup,
		shardDagIndexCache: shardedDagIndexCache,
		providerCacher:     providerCacher,
	}
}

func (b *cachingLookup) Find(ctx context.Context, contextID types.EncodedContextID, provider model.ProviderResult, fetchURL url.URL, rng *metadata.Range) (blobindex.ShardedDagIndexView, error) {
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
	if err := b.providerCacher.CacheProviderForIndex(ctx, provider, index); err != nil {
		return nil, fmt.Errorf("queueing provider caching for index failed: %w", err)
	}

	return index, nil
}
