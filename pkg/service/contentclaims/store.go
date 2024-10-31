package contentclaims

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/ipfs/go-datastore"
	"github.com/ipld/go-ipld-prime"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/indexing-service/pkg/types"
	"github.com/storacha/ipni-publisher/pkg/store"
)

type bucketStore struct {
	bucket store.Store
}

func (bs *bucketStore) Get(ctx context.Context, key ipld.Link) (delegation.Delegation, error) {
	r, err := bs.bucket.Get(ctx, toKey(key))
	if err != nil {
		if store.IsNotFound(err) {
			return nil, types.ErrKeyNotFound
		}
		return nil, err
	}
	defer r.Close()

	b, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return delegation.Extract(b)
}

func (bs *bucketStore) Put(ctx context.Context, key ipld.Link, value delegation.Delegation) error {
	return bs.bucket.Put(ctx, toKey(key), value.Archive())
}

var _ types.ContentClaimsStore = (*bucketStore)(nil)

// NewStoreFromBucket creates a claims store from a bucket style interface.
func NewStoreFromBucket(bucket store.Store) types.ContentClaimsStore {
	return &bucketStore{bucket}
}

type dsStore struct {
	ds datastore.Datastore
}

func (d *dsStore) Get(ctx context.Context, key ipld.Link) (delegation.Delegation, error) {
	b, err := d.ds.Get(ctx, datastore.NewKey(toKey(key)))
	if err != nil {
		if errors.Is(err, datastore.ErrNotFound) {
			return nil, types.ErrKeyNotFound
		}
		return nil, err
	}
	return delegation.Extract(b)

}

func (d *dsStore) Put(ctx context.Context, key ipld.Link, value delegation.Delegation) error {
	r := value.Archive()
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	return d.ds.Put(ctx, datastore.NewKey(toKey(key)), b)
}

var _ types.ContentClaimsStore = (*dsStore)(nil)

func NewStoreFromDatastore(ds datastore.Datastore) types.ContentClaimsStore {
	return &dsStore{ds}
}

// toKey transforms the claim root CID into a string key.
func toKey(link ipld.Link) string {
	return fmt.Sprintf("%s/%s.car", link, link)
}
