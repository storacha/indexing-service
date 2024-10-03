package blobindexlookup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/ipni/go-libipni/find/model"
	"github.com/storacha/indexing-service/pkg/blobindex"
	"github.com/storacha/indexing-service/pkg/metadata"
	"github.com/storacha/indexing-service/pkg/types"
)

type IndexProviderCacher interface {
	CacheProviderForIndexRecords(ctx context.Context, provider model.ProviderResult, index blobindex.ShardedDagIndexView) error
}

type BlobIndexLookup struct {
	shardDagIndexCache  types.ShardedDagIndexStore
	providerStore       types.ProviderStore
	httpClient          *http.Client
	indexProviderCacher IndexProviderCacher
}

func NewBlobIndex(shardedDagIndexCache types.ShardedDagIndexStore, providerStore types.ProviderStore, httpClient *http.Client, indexProviderCacher IndexProviderCacher) *BlobIndexLookup {
	return &BlobIndexLookup{
		shardDagIndexCache:  shardedDagIndexCache,
		providerStore:       providerStore,
		httpClient:          httpClient,
		indexProviderCacher: indexProviderCacher,
	}
}

func (b *BlobIndexLookup) Find(ctx context.Context, contextID types.EncodedContextID, provider model.ProviderResult, fetchURL url.URL, rng *metadata.Range) (blobindex.ShardedDagIndexView, error) {
	// attempt to read index from cache and return it if succesful
	index, err := b.shardDagIndexCache.Get(ctx, contextID)
	if err == nil {
		return index, nil
	}

	// if an error occurred other than the claim not being in the cache, return it
	if !errors.Is(err, types.ErrKeyNotFound) {
		return nil, fmt.Errorf("reading from index cache: %w", err)
	}

	// attempt to fetch the index from provided url
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fetchURL.String(), nil)
	if rng != nil {
		rangeHeader := fmt.Sprintf("bytes=%d-", rng.Offset)
		if rng.Length != nil {
			rangeHeader += strconv.FormatUint(rng.Offset+*rng.Length-1, 10)
		}
		req.Header.Set("Range", rangeHeader)
	}
	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch index: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := io.ReadAll(resp.Body)

		return nil, fmt.Errorf("failure response fetching index. status: %s, message: %s", resp.Status, string(body))
	}
	index, err = blobindex.Extract(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("extracting index from response: %w", err)
	}

	// cache the index for the future
	if err := b.shardDagIndexCache.Set(ctx, contextID, index, true); err != nil {
		return nil, fmt.Errorf("caching fetched index: %w", err)
	}

	// queue a background cache of an index record for all cids in the cache without one
	b.indexProviderCacher.CacheProviderForIndexRecords(ctx, provider, index)

	return index, nil
}
