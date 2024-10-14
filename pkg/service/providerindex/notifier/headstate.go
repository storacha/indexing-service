package notifier

import (
	"context"
	"errors"
	"fmt"

	cid "github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/storacha/go-ucanto/core/ipld"
)

var remoteHeadPrefix = datastore.NewKey("head/remote/")

type HeadState struct {
	ds     datastore.Batching
	hdkey  datastore.Key
	cached ipld.Link
}

func NewHeadState(ds datastore.Batching, hostname string) (*HeadState, error) {

	var hd ipld.Link
	hdkey := remoteHeadPrefix.ChildString(hostname)
	v, err := ds.Get(context.Background(), hdkey)
	if err != nil {
		if !errors.Is(err, datastore.ErrNotFound) {
			return nil, fmt.Errorf("getting remote IPNI head CID from datastore: %w", err)
		}
	} else {
		c, err := cid.Cast(v)
		if err != nil {
			return nil, fmt.Errorf("parsing remote IPNI head CID: %w", err)
		}
		hd = cidlink.Link{Cid: c}
	}
	return &HeadState{ds: ds, cached: hd}, nil
}

func (h *HeadState) Get(ctx context.Context) ipld.Link {
	return h.cached
}

func (h *HeadState) Set(ctx context.Context, head ipld.Link) error {
	err := h.ds.Put(ctx, h.hdkey, []byte(head.Binary()))
	if err != nil {
		return fmt.Errorf("saving remote IPNI sync'd head: %w", err)
	}
	h.cached = head
	return nil
}
