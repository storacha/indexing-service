package providerindex

import (
	"context"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	mh "github.com/multiformats/go-multihash"
	"github.com/storacha/indexing-service/pkg/internal/jobqueue"
	"github.com/storacha/indexing-service/pkg/service/providerindex/publisher"
	"github.com/storacha/indexing-service/pkg/types"
)

type RemoteSyncer struct {
	providerStore types.ProviderStore
	publisher     publisher.Publisher
}

func NewRemoteSyncer(providerStore types.ProviderStore, publisher publisher.Publisher) *RemoteSyncer {
	return &RemoteSyncer{
		providerStore: providerStore,
		publisher:     publisher,
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
		ad, err := rs.publisher.Store().Advert(ctx, cur)
		if err != nil {
			log.Errorf("getting advert: %s: %w", cur, err)
			return
		}
		for d, err := range rs.publisher.Store().Entries(ctx, ad.Entries) {
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
		if ad.PreviousCid() == cid.Undef || ad.PreviousCid().String() == prev.String() {
			break
		}
		cur = cidlink.Link{Cid: ad.PreviousCid()}
	}

	err := q.Shutdown(ctx)
	if err != nil {
		log.Errorf("shutting down IPNI remote sync job queue: %w", err)
	}
	log.Infof("handled IPNI remote sync from %s to %s", prev, head)
}
