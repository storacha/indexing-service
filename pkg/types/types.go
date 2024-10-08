package types

import (
	"bytes"
	"context"
	"errors"

	"github.com/ipfs/go-cid"
	"github.com/ipni/go-libipni/find/model"
	mh "github.com/multiformats/go-multihash"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/go-ucanto/did"
	"github.com/storacha/indexing-service/pkg/blobindex"
)

// ContextID describes the data used to calculate a context id for IPNI
type ContextID struct {
	Space *did.DID
	Hash  mh.Multihash
}

// EncodedContextID is the encoded form of context id data that is actually stored
// in IPNI
type EncodedContextID []byte

// ToEncoded canonically encodes ContextID data
func (c ContextID) ToEncoded() (EncodedContextID, error) {
	if c.Space == nil {
		return EncodedContextID(c.Hash), nil
	}
	mh, err := mh.Sum(bytes.Join([][]byte{c.Space.Bytes(), c.Hash}, nil), mh.SHA2_256, -1)
	return EncodedContextID(mh), err
}

// ErrKeyNotFound means the key did not exist in the cache
var ErrKeyNotFound = errors.New("cache key not found")

// Cache describes a generic cache interface
type Cache[Key, Value any] interface {
	Set(ctx context.Context, key Key, value Value, expires bool) error
	SetExpirable(ctx context.Context, key Key, expires bool) error
	Get(ctx context.Context, key Key) (Value, error)
}

// ProviderStore caches queries to IPNI
type ProviderStore Cache[mh.Multihash, []model.ProviderResult]

// ContentClaimsStore caches fetched content claims
type ContentClaimsStore Cache[cid.Cid, delegation.Delegation]

// ShardedDagIndexStore caches fetched sharded dag indexes
type ShardedDagIndexStore Cache[EncodedContextID, blobindex.ShardedDagIndexView]
