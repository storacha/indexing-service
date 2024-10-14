package publisher

import (
	"context"
	"fmt"
	"sync"

	cid "github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	logging "github.com/ipfs/go-log/v2"
	"github.com/ipld/go-ipld-prime"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/ipld/go-ipld-prime/storage/dsadapter"
	"github.com/ipni/go-libipni/metadata"
	provider "github.com/ipni/index-provider"
	"github.com/ipni/index-provider/engine"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	mh "github.com/multiformats/go-multihash"
)

var log = logging.Logger("publisher")
var remoteHeadPrefix = datastore.NewKey("head/remote/")

// The below definitions allow us to actually read entry blocks from the
// IPNI datastore. For some reason there's not accessor for this.
// https://github.com/ipni/index-provider/blob/69eb98045424f6074fc351b9d4771d0725a28620/engine/engine.go#L39
var entriesNamespace = datastore.NewKey("/cache/links")

// maxEntryChunkSize is the maximum number of multihashes each advertisement
// entry chunk may contain.
var maxEntryChunkSize = 16384

type Publisher interface {
	// Publish publishes an advert to indexer(s). Note: it is not necessary to
	// sign the advert - this is done automatically.
	Publish(ctx context.Context, provider *peer.AddrInfo, contextID string, digests []mh.Multihash, meta metadata.Metadata) (ipld.Link, error)
	// Store returns the storage interface used to access published data.
	Store() AdvertStore
}

type IPNIPublisher struct {
	engine *engine.Engine
	store  AdvertStore
	mutex  sync.RWMutex
}

func (p *IPNIPublisher) Start(ctx context.Context) error {
	err := p.engine.Start(ctx)
	if err != nil {
		return fmt.Errorf("starting IPNI index provider: %w", err)
	}
	return nil
}

func (p *IPNIPublisher) Publish(ctx context.Context, providerInfo *peer.AddrInfo, contextID string, digests []mh.Multihash, meta metadata.Metadata) (ipld.Link, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	// such a weird API
	p.engine.RegisterMultihashLister(func(ctx context.Context, _ peer.ID, _ []byte) (provider.MultihashIterator, error) {
		return provider.SliceMultihashIterator(digests), nil
	})
	cid, err := p.engine.NotifyPut(ctx, providerInfo, []byte(contextID), meta)
	if err != nil {
		return nil, fmt.Errorf("publishing IPNI advert: %w", err)
	}
	return cidlink.Link{Cid: cid}, nil
}

func (p *IPNIPublisher) Store() AdvertStore {
	return p.store
}

func (p *IPNIPublisher) Shutdown() error {
	err := p.engine.Shutdown()
	return err
}

func New(id crypto.PrivKey, opts ...Option) (*IPNIPublisher, error) {
	o := &options{}
	for _, opt := range opts {
		err := opt(o)
		if err != nil {
			return nil, err
		}
	}

	ds := o.datastore
	if ds == nil {
		log.Warnf("datastore not configured, using in-memory store")
		ds = datastore.NewMapDatastore()
	}

	listenAddr := o.listenAddr
	if listenAddr == "" {
		log.Warnf(`no HTTP listen address configured, using default "0.0.0.0:3104"`)
		listenAddr = "0.0.0.0:3104"
	}

	// not sure why we need a libp2p host, but we get this error otherwise:
	// > Libp2p host is not configured, but required; created a new host.
	h, err := libp2p.New(libp2p.Identity(id))
	if err != nil {
		return nil, fmt.Errorf("creating libp2p host: %w", err)
	}

	engopts := []engine.Option{
		engine.WithPrivateKey(id),
		engine.WithHost(h),
		engine.WithPublisherKind(engine.HttpPublisher),
		engine.WithHttpPublisherListenAddr(listenAddr),
		engine.WithDatastore(ds),
		engine.WithChainedEntries(maxEntryChunkSize),
	}
	for _, addr := range o.announceAddrs {
		engopts = append(engopts, engine.WithHttpPublisherAnnounceAddr(addr))
	}
	if len(o.announceAddrs) == 0 {
		log.Warnf("no HTTP address(es) configured for announcements")
	}

	engine, err := engine.New(engopts...)
	if err != nil {
		return nil, fmt.Errorf("creating IPNI index provider: %w", err)
	}

	ads := NewAdvertStore(&dsadapter.Adapter{Wrapped: ds}, &dsadapter.Adapter{Wrapped: namespace.Wrap(ds, entriesNamespace)})
	return &IPNIPublisher{engine: engine, store: ads}, nil
}

func asCID(link ipld.Link) cid.Cid {
	if cl, ok := link.(cidlink.Link); ok {
		return cl.Cid
	}
	return cid.MustParse(link.String())
}
