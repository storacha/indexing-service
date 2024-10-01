package blobindexlookup

import (
	"context"
	"errors"
	"net/url"

	"github.com/storacha-network/indexing-service/pkg/blobindex"
	"github.com/storacha-network/indexing-service/pkg/metadata"
	"github.com/storacha-network/indexing-service/pkg/types"
)

type BlobIndexLookup struct {
	blobCache types.ShardedDagIndexStore
	ipniCache types.IPNIStore
}

func NewBlobIndex(blobCache types.ShardedDagIndexStore, ipniCache types.IPNIStore) *BlobIndexLookup {
	return &BlobIndexLookup{
		blobCache: blobCache,
		ipniCache: ipniCache,
	}
}

func (b *BlobIndexLookup) Find(ctx context.Context, contextId types.EncodedContextID, fetchURL url.URL, rng *metadata.Range) (blobindex.ShardedDagIndexView, error) {
	return nil, errors.New("not implemented")
}
