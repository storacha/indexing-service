package providercacher

import (
	"context"
	"fmt"

	"github.com/ipni/go-libipni/find/model"
	"github.com/storacha/go-libstoracha/blobindex"
	"github.com/storacha/indexing-service/pkg/internal/link"
	"github.com/storacha/indexing-service/pkg/telemetry"
	"github.com/storacha/indexing-service/pkg/types"
	"go.opentelemetry.io/otel/attribute"
)

// MaxBatchSize is the maximum number of items that'll be added to a batch.
const MaxBatchSize = 10_000

type simpleProviderCacher struct {
	providerStore types.ProviderStore
}

func NewSimpleProviderCacher(providerStore types.ProviderStore) ProviderCacher {
	return &simpleProviderCacher{providerStore: providerStore}
}

func (s *simpleProviderCacher) CacheProviderForIndexRecords(ctx context.Context, provider model.ProviderResult, index blobindex.ShardedDagIndexView) error {
	ctx, span := telemetry.StartSpan(ctx, "ProviderCacher.CacheProviderForIndexRecords")
	defer span.End()

	batch := s.providerStore.Batch()

	// Prioritize the root
	rootDigest := link.ToCID(index.Content()).Hash()
	err := batch.Add(ctx, rootDigest, provider)
	if err != nil {
		return fmt.Errorf("batch adding provider for root: %w", err)
	}
	err = batch.SetExpirable(ctx, rootDigest, true)
	if err != nil {
		return fmt.Errorf("batch setting provider expirable for root: %w", err)
	}

	total := 0
	size := 1
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
			total++
			size++
			if size >= MaxBatchSize {
				span.AddEvent("commit batch")
				err := batch.Commit(ctx)
				if err != nil {
					return fmt.Errorf("batch commiting: %w", err)
				}
				batch = s.providerStore.Batch()
				size = 0
			}
		}
	}
	span.SetAttributes(attribute.KeyValue{Key: "total", Value: attribute.IntValue(total)})
	if size == 0 {
		return nil
	}
	span.AddEvent("commit batch")
	return batch.Commit(ctx)
}
