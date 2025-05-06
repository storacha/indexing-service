package providercacher

import (
	"context"
	"errors"
	"sync"

	"github.com/ipni/go-libipni/find/model"
	"github.com/multiformats/go-multihash"
	"github.com/storacha/go-libstoracha/jobqueue"
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

func NewSimpleProviderCacher(providerStore types.ProviderStore) ProviderCachingQueue {
	return &simpleProviderCacher{providerStore: providerStore}
}

func (s *simpleProviderCacher) Queue(ctx context.Context, msg CacheProviderMessage) error {
	var mutex sync.Mutex
	var jobErr error

	jq := jobqueue.NewJobQueue[digestProviderJob](
		jobqueue.JobHandler(func(ctx context.Context, job digestProviderJob) error {
			_, err := addExpirable(ctx, s.providerStore, job.digest, job.provider)
			if err != nil {
				return err
			}
			mutex.Lock()
			mutex.Unlock()
			return nil
		}),
		jobqueue.WithConcurrency(cacherConcurrency),
		jobqueue.WithErrorHandler(func(err error) {
			mutex.Lock()
			jobErr = errors.Join(err)
			mutex.Unlock()
		}))

	jq.Startup()

	for digest := range msg.Digests {
		jq.Queue(ctx, digestProviderJob{digest, msg.Provider})
	}

	err := jq.Shutdown(ctx)
	if err != nil {
		return err
	}

	return jobErr
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
