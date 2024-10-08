package providercacher

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/ipni/go-libipni/find/model"
	"github.com/storacha/indexing-service/pkg/blobindex"
	"github.com/storacha/indexing-service/pkg/providerresults"
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
			existing, err := s.providerStore.Get(ctx, hash)
			if err != nil && !errors.Is(err, types.ErrKeyNotFound) {
				return written, err
			}

			inList := slices.ContainsFunc(existing, func(matchProvider model.ProviderResult) bool {
				fmt.Println(provider, matchProvider)
				return providerresults.Equals(provider, matchProvider)
			})
			if !inList {
				newResults := append(existing, provider)
				err = s.providerStore.Set(ctx, hash, newResults, true)
				if err != nil {
					return written, err
				}
				written++
			}
		}
	}
	return written, nil
}
