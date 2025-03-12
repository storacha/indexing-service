package providercacher

import (
	"context"

	"github.com/ipni/go-libipni/find/model"
	"github.com/multiformats/go-multihash"
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
	written := uint64(0)

	// Prioritize the root
	rootDigest := link.ToCID(index.Content()).Hash()
	err := addExpirable(ctx, s.providerStore, rootDigest, provider)
	if err != nil {
		return written, err
	}
	written++

	for _, shardIndex := range index.Shards().Iterator() {
		for hash := range shardIndex.Iterator() {
			if string(hash) == string(rootDigest) {
				continue // already added
			}
			err := addExpirable(ctx, s.providerStore, hash, provider)
			if err != nil {
				return written, err
			}
			written++
		}
	}
	return written, nil
}

func addExpirable(ctx context.Context, providerStore types.ProviderStore, digest multihash.Multihash, provider model.ProviderResult) error {
	n, err := providerStore.Add(ctx, digest, provider)
	if err != nil {
		return err
	}
	if n > 0 {
		err = providerStore.SetExpirable(ctx, digest, true)
		if err != nil {
			return err
		}
	}
	return nil
}
