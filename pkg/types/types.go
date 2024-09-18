package types

import (
	"bytes"

	"github.com/ipfs/go-cid"
	"github.com/ipni/go-libipni/find/model"
	mh "github.com/multiformats/go-multihash"
	"github.com/storacha-network/go-ucanto/did"
	"github.com/storacha-network/indexing-service/pkg/blobindex"
	"github.com/web3-storage/go-ucanto/core/delegation"
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

// Cache describes a generic cache interface
type Cache[Key, Value any] interface {
	Set(key Key, value Value, expires bool) error
	SetExpirable(key Key, expires bool)
	Get(key Key) (Value, error)
}

// IPNIStore caches queries to IPNI
type IPNIStore Cache[mh.Multihash, []model.ProviderResult]

// ContentClaimsStore caches fetched content claims
type ContentClaimsStore Cache[cid.Cid, delegation.Delegation]

// ShardedDagIndexStore caches fetched sharded dag indexes
type ShardedDagIndexStore Cache[EncodedContextID, blobindex.ShardedDagIndexView]
