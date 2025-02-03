package providercacher

import (
	"context"

	"github.com/ipni/go-libipni/find/model"
	"github.com/storacha/indexing-service/pkg/blobindex"
	"github.com/storacha/indexing-service/pkg/types"
)

type simpleProviderCacher struct {
	providerStore types.ProviderStore
}

func NewSimpleProviderCacher(providerStore types.ProviderStore) ProviderCacher {
	return &simpleProviderCacher{providerStore: providerStore}
}

func (s *simpleProviderCacher) CacheProviderForIndexRecords(ctx context.Context, provider model.ProviderResult, index blobindex.ShardedDagIndexView) (uint64, error) {
	written := uint64(0)
	for _, shardIndex := range index.Shards().Iterator() {
		for hash := range shardIndex.Iterator() {
			n, err := s.providerStore.Add(ctx, hash, provider)
			written += n
			if err != nil {
				return written, err
			}
			if n > 0 {
				err = s.providerStore.SetExpirable(ctx, hash, true)
				if err != nil {
					return written, err
				}
			}
		}
	}
	return written, nil
}
