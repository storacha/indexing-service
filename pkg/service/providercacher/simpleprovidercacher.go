package providercacher

import (
	"context"
	"errors"
	"sync"

	"github.com/ipni/go-libipni/find/model"
	"github.com/multiformats/go-multihash"
	"github.com/storacha/go-libstoracha/jobqueue"
	"github.com/storacha/indexing-service/pkg/blobindex"
	"github.com/storacha/indexing-service/pkg/internal/link"
	"github.com/storacha/indexing-service/pkg/types"
)

var cacherConcurrency = 5

type digestProviderJob struct {
	digest   multihash.Multihash
	provider model.ProviderResult
}

type simpleProviderCacher struct {
	providerStore types.ProviderStore
}

func NewSimpleProviderCacher(providerStore types.ProviderStore) ProviderCacher {
	return &simpleProviderCacher{providerStore: providerStore}
}

func (s *simpleProviderCacher) CacheProviderForIndexRecords(ctx context.Context, provider model.ProviderResult, index blobindex.ShardedDagIndexView) (uint64, error) {
	var mutex sync.Mutex
	var written uint64
	var jobErr error

	jq := jobqueue.NewJobQueue[digestProviderJob](
		jobqueue.JobHandler(func(ctx context.Context, job digestProviderJob) error {
			n, err := addExpirable(ctx, s.providerStore, job.digest, job.provider)
			if err != nil {
				return err
			}
			mutex.Lock()
			written += n
			mutex.Unlock()
			return nil
		}),
		jobqueue.WithBuffer(5),
		jobqueue.WithConcurrency(cacherConcurrency),
		jobqueue.WithErrorHandler(func(err error) {
			mutex.Lock()
			jobErr = errors.Join(err)
			mutex.Unlock()
		}))

	jq.Startup()

	// Prioritize the root
	rootDigest := link.ToCID(index.Content()).Hash()
	jq.Queue(ctx, digestProviderJob{rootDigest, provider})

	for _, shardIndex := range index.Shards().Iterator() {
		for hash := range shardIndex.Iterator() {
			if string(hash) == string(rootDigest) {
				continue // already added
			}
			jq.Queue(ctx, digestProviderJob{hash, provider})
		}
	}

	err := jq.Shutdown(ctx)
	if err != nil {
		return written, err
	}

	return written, jobErr
}

func addExpirable(ctx context.Context, providerStore types.ProviderStore, digest multihash.Multihash, provider model.ProviderResult) (uint64, error) {
	n, err := providerStore.Add(ctx, digest, provider)
	if err != nil {
		return n, err
	}
	if n > 0 {
		err = providerStore.SetExpirable(ctx, digest, true)
		if err != nil {
			return n, err
		}
	}
	return n, nil
}
