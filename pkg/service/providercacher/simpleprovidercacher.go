package providercacher

import (
	"context"
	"fmt"

	"github.com/ipni/go-libipni/find/model"
	"github.com/storacha/indexing-service/pkg/blobindex"
	"github.com/storacha/indexing-service/pkg/internal/link"
	"github.com/storacha/indexing-service/pkg/types"
)

type simpleProviderCacher struct {
	providerStore types.ProviderStore
}

func NewSimpleProviderCacher(providerStore types.ProviderStore) ProviderCacher {
	return &simpleProviderCacher{providerStore: providerStore}
}

func (s *simpleProviderCacher) CacheProviderForIndexRecords(ctx context.Context, provider model.ProviderResult, index blobindex.ShardedDagIndexView) error {
	batch, err := s.providerStore.Batch()
	if err != nil {
		return fmt.Errorf("creating batch: %w", err)
	}

	// Prioritize the root
	rootDigest := link.ToCID(index.Content()).Hash()
	err = batch.Add(ctx, rootDigest, provider)
	if err != nil {
		return fmt.Errorf("batch adding provider for root: %w", err)
	}
	err = batch.SetExpirable(ctx, rootDigest, true)
	if err != nil {
		return fmt.Errorf("batch setting provider expirable for root: %w", err)
	}

	for _, shardIndex := range index.Shards().Iterator() {
		for hash := range shardIndex.Iterator() {
			if string(hash) == string(rootDigest) {
				continue // already added
			}
			err := batch.Add(ctx, hash, provider)
			if err != nil {
				return fmt.Errorf("batch adding provider: %w", err)
			}
			err = batch.SetExpirable(ctx, hash, true)
			if err != nil {
				return fmt.Errorf("batch setting provider expirable: %w", err)
			}
		}
	}

	return batch.Commit(ctx)
}
