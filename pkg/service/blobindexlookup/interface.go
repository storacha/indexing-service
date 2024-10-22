package blobindexlookup

import (
	"context"
	"net/url"

	"github.com/ipni/go-libipni/find/model"
	"github.com/storacha/go-metadata"
	"github.com/storacha/indexing-service/pkg/blobindex"
	"github.com/storacha/indexing-service/pkg/types"
)

type BlobIndexLookup interface {
	Find(ctx context.Context, contextID types.EncodedContextID, provider model.ProviderResult, fetchURL url.URL, rng *metadata.Range) (blobindex.ShardedDagIndexView, error)
}
