package providercacher

import (
	"context"

	"github.com/ipni/go-libipni/find/model"
	"github.com/storacha/indexing-service/pkg/blobindex"
)

type ProviderCacher interface {
	CacheProviderForIndexRecords(ctx context.Context, provider model.ProviderResult, index blobindex.ShardedDagIndexView) error
}
