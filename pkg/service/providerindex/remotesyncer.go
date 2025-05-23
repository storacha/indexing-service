package providerindex

import (
	"context"

	"github.com/ipld/go-ipld-prime"
	"github.com/storacha/go-libstoracha/ipnipublisher/store"
	"github.com/storacha/indexing-service/pkg/types"
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

	cur := head
	for {
		ad, err := rs.store.Advert(ctx, cur)
		if err != nil {
			log.Errorf("getting advert: %s: %w", cur, err)
			return
		}
		batch := rs.providerStore.Batch()
		for d, err := range rs.store.Entries(ctx, ad.Entries) {
			if err != nil {
				log.Errorf("iterating advert entries: %s (advert) -> %s (entries): %w", cur, ad.Entries, err)
				return
			}
			err := batch.SetExpirable(ctx, d, true)
			if err != nil {
				log.Errorf("adding digest to batch: %s: %w", d.B58String(), err)
				return
			}
		}
		err = batch.Commit(ctx)
		if err != nil {
			log.Errorf("comitting batch: %s: %w", cur.String(), err)
			return
		}
		if ad.PreviousID == nil || (prev != nil && ad.PreviousID.String() == prev.String()) {
			break
		}
		cur = ad.PreviousID
	}

	log.Infof("handled IPNI remote sync from %s to %s", prev, head)
}
