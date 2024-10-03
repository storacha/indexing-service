package indexprovidercacher

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"sync"

	"github.com/ipni/go-libipni/find/model"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/storacha/indexing-service/pkg/blobindex"
	"github.com/storacha/indexing-service/pkg/types"
)

type (
	cacheProviderJob struct {
		provider model.ProviderResult
		index    blobindex.ShardedDagIndexView
	}

	jobComplete struct {
		provider model.ProviderResult
		index    blobindex.ShardedDagIndexView
		written  uint64
		err      error
	}

	IndexProviderCacher struct {
		*config
		providerStore types.ProviderStore
		incoming      chan cacheProviderJob
	}

	CacheCompleteHook func(provider model.ProviderResult, index blobindex.ShardedDagIndexView, written uint64, err error)
	config            struct {
		buffer            int
		concurrency       int
		cacheCompleteHook CacheCompleteHook
	}

	Option func(*config)
)

func WithBuffer(buffer int) Option {
	return func(c *config) {
		c.buffer = buffer
	}
}

func WithConcurrency(concurrency int) Option {
	return func(c *config) {
		c.concurrency = concurrency
	}
}

func WithCacheCompleteHook(cacheCompleteHook CacheCompleteHook) Option {
	return func(c *config) {
		c.cacheCompleteHook = cacheCompleteHook
	}
}

func NewIndexProviderCache(providerStore types.ProviderStore, opts ...Option) *IndexProviderCacher {
	c := &config{
		buffer:      0,
		concurrency: 1,
	}
	for _, opt := range opts {
		opt(c)
	}
	return &IndexProviderCacher{
		config:        c,
		providerStore: providerStore,
		incoming:      make(chan cacheProviderJob, c.buffer),
	}
}

func (i *IndexProviderCacher) CacheProviderForIndexRecords(ctx context.Context, provider model.ProviderResult, index blobindex.ShardedDagIndexView) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case i.incoming <- cacheProviderJob{provider, index}:
		return nil
	}
}

func (i *IndexProviderCacher) Run(ctx context.Context) {
	var wg sync.WaitGroup
	completes := make(chan jobComplete)
	for range i.concurrency {
		wg.Add(1)
		go func() {
			i.worker(ctx, completes)
			wg.Done()
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case jc := <-completes:
				if i.cacheCompleteHook != nil {
					i.cacheCompleteHook(jc.provider, jc.index, jc.written, jc.err)
				}
			}
		}
	}()
	wg.Wait()
}

func (i *IndexProviderCacher) worker(ctx context.Context, completes chan<- jobComplete) {
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-i.incoming:
			written, err := i.cacheProviderForIndexRecords(ctx, job.provider, job.index)
			select {
			case completes <- jobComplete{job.provider, job.index, written, err}:
			case <-ctx.Done():
				return
			}
		}
	}
}

func (i *IndexProviderCacher) cacheProviderForIndexRecords(ctx context.Context, provider model.ProviderResult, index blobindex.ShardedDagIndexView) (uint64, error) {
	written := uint64(0)
	for _, shardIndex := range index.Shards().Iterator() {
		for hash := range shardIndex.Iterator() {
			existing, err := i.providerStore.Get(ctx, hash)
			if err != nil && !errors.Is(err, types.ErrKeyNotFound) {
				return written, err
			}
			inList := slices.ContainsFunc(existing, func(matchProvider model.ProviderResult) bool { return equalProviderResult(provider, matchProvider) })
			if !inList {
				newResults := append(existing, provider)
				err = i.providerStore.Set(ctx, hash, newResults, true)
				if err != nil {
					return written, err
				}
				written++
			}
		}
	}
	return written, nil
}

func equalProvider(a, b *peer.AddrInfo) bool {
	if a == nil {
		return b == nil
	}
	return b != nil && a.String() == b.String()
}

func equalProviderResult(a, b model.ProviderResult) bool {
	return bytes.Equal(a.ContextID, b.ContextID) &&
		bytes.Equal(a.Metadata, b.Metadata) &&
		equalProvider(a.Provider, b.Provider)
}
