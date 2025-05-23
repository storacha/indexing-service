package providercacher

import (
	"context"

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

func (s *simpleProviderCacher) CacheProviderForIndexRecords(ctx context.Context, provider model.ProviderResult, index blobindex.ShardedDagIndexView) (uint64, error) {
	batch := s.providerStore.Batch()

	// Prioritize the root
	rootDigest := link.ToCID(index.Content()).Hash()
	err := batch.Add(ctx, rootDigest, provider)
	if err != nil {
		return 0, err
	}
	err = batch.SetExpirable(ctx, rootDigest, true)
	if err != nil {
		return 0, err
	}

	for _, shardIndex := range index.Shards().Iterator() {
		for hash := range shardIndex.Iterator() {
			if string(hash) == string(rootDigest) {
				continue // already added
			}
			err := batch.Add(ctx, hash, provider)
			if err != nil {
				return 0, err
			}
			err = batch.SetExpirable(ctx, hash, true)
			if err != nil {
				return 0, err
			}
		}
	}

	err = batch.Commit(ctx)
	if err != nil {
		return 0, err
	}

	return 0, nil
}
