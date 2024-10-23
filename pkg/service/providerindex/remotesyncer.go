package providerindex

import (
	"context"

	"github.com/ipld/go-ipld-prime"
	mh "github.com/multiformats/go-multihash"
	"github.com/storacha/indexing-service/pkg/internal/jobqueue"
	"github.com/storacha/indexing-service/pkg/types"
	"github.com/storacha/ipni-publisher/pkg/store"
)

type Store interface {
	store.EntriesReadable
	store.AdvertReadable
}

type RemoteSyncer struct {
	providerStore types.ProviderStore
	store         Store
}

func NewRemoteSyncer(providerStore types.ProviderStore, store Store) *RemoteSyncer {
	return &RemoteSyncer{
		providerStore: providerStore,
		store:         store,
	}
}

func (rs *RemoteSyncer) HandleRemoteSync(ctx context.Context, head, prev ipld.Link) {
	log.Infof("handling IPNI remote sync from %s to %s", prev, head)

	q := jobqueue.NewJobQueue(
		func(ctx context.Context, digest mh.Multihash) error {
			return rs.providerStore.SetExpirable(ctx, digest, true)
		},
		jobqueue.WithConcurrency(5),
		jobqueue.WithErrorHandler(func(err error) {
			log.Errorf("setting expirable: %w", err)
		}),
	)
	q.Startup()

	cur := head
	for {
		ad, err := rs.store.Advert(ctx, cur)
		if err != nil {
			log.Errorf("getting advert: %s: %w", cur, err)
			return
		}
		for d, err := range rs.store.Entries(ctx, ad.Entries) {
			if err != nil {
				log.Errorf("iterating advert entries: %s (advert) -> %s (entries): %w", cur, ad.Entries, err)
				return
			}
			err := q.Queue(ctx, d)
			if err != nil {
				log.Errorf("adding digest to queue: %s: %w", d.B58String(), err)
				return
			}
		}
		if ad.PreviousID == nil || (prev != nil && ad.PreviousID.String() == prev.String()) {
			break
		}
		cur = ad.PreviousID
	}

	err := q.Shutdown(ctx)
	if err != nil {
		log.Errorf("shutting down IPNI remote sync job queue: %w", err)
	}
	log.Infof("handled IPNI remote sync from %s to %s", prev, head)
}
