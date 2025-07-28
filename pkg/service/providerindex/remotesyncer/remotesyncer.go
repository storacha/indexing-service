package remotesyncer

import (
	"context"

	logging "github.com/ipfs/go-log/v2"
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
	log           logging.EventLogger
}

type config struct {
	log logging.EventLogger
}

// Option configures the RemoteSyncer.
type Option func(conf *config)

// WithLogger configures the service to use the passed logger instead of the
// default logger.
func WithLogger(log logging.EventLogger) Option {
	return func(conf *config) {
		conf.log = log
	}
}

func New(providerStore types.ProviderStore, store Store, options ...Option) *RemoteSyncer {
	conf := config{}
	for _, option := range options {
		option(&conf)
	}
	if conf.log == nil {
		conf.log = logging.Logger("remotesyncer")
	}
	return &RemoteSyncer{
		providerStore: providerStore,
		store:         store,
		log:           conf.log,
	}
}

func (rs *RemoteSyncer) HandleRemoteSync(ctx context.Context, head, prev ipld.Link) {
	rs.log.Infof("handling IPNI remote sync from %s to %s", prev, head)

	cur := head
	for {
		ad, err := rs.store.Advert(ctx, cur)
		if err != nil {
			rs.log.Errorf("getting advert: %s: %s", cur, err)
			return
		}
		batch := rs.providerStore.Batch()
		for d, err := range rs.store.Entries(ctx, ad.Entries) {
			if err != nil {
				rs.log.Errorf("iterating advert entries: %s (advert) -> %s (entries): %s", cur, ad.Entries, err)
				return
			}
			err := batch.SetExpirable(ctx, d, true)
			if err != nil {
				rs.log.Errorf("adding digest to batch: %s: %s", d.B58String(), err)
				return
			}
		}
		err = batch.Commit(ctx)
		if err != nil {
			rs.log.Errorf("comitting batch: %s: %s", cur.String(), err)
			return
		}
		if ad.PreviousID == nil || (prev != nil && ad.PreviousID.String() == prev.String()) {
			break
		}
		cur = ad.PreviousID
	}

	rs.log.Infof("handled IPNI remote sync from %s to %s", prev, head)
}
