package storage

import (
	"github.com/ipni/go-libipni/find/model"
	mh "github.com/multiformats/go-multihash"
	"github.com/storacha-network/indexing-service/pkg/blobindex"
)

type EncodedContextID []byte

type IPNIStore interface {
	Set(mh.Multihash, []model.ProviderResult) error
	GetAll(mh.Multihash) ([]model.ProviderResult, error)
}

type ShardedDagIndexStore interface {
	Set(EncodedContextID, blobindex.ShardedDagIndexView) error
	Get(EncodedContextID) (blobindex.ShardedDagIndexView, error)
}

type LocationBundle interface{}

type ShardLocationBundleStore interface {
	Set(EncodedContextID, LocationBundle)
	Get(EncodedContextID, LocationBundle)
}
