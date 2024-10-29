package types

import (
	"bytes"
	"context"
	"errors"

	"github.com/ipfs/go-cid"
	"github.com/ipni/go-libipni/find/model"
	"github.com/libp2p/go-libp2p/core/peer"
	mh "github.com/multiformats/go-multihash"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/go-ucanto/core/ipld"
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
var ErrKeyNotFound = errors.New("key not found")

// Store describes a generic storage interface
type Store[Key, Value any] interface {
	// Put adds (or replaces) an item in the store.
	Put(ctx context.Context, key Key, value Value) error
	// Get retrieves an existing item from the store. If the item does not exist,
	// it should return [ErrKeyNotFound].
	Get(ctx context.Context, key Key) (Value, error)
}

// ErrWrongRootCount indicates a car file with multiple roots being unable to interpret
// as a query result
var ErrWrongRootCount = errors.New("query result should have exactly one root")

// ErrNoRootBlock indicates a root that is specified but not found in a CAR file
var ErrNoRootBlock = errors.New("query root block not found in car")

// Cache describes a generic cache interface
type Cache[Key, Value any] interface {
	Set(ctx context.Context, key Key, value Value, expires bool) error
	SetExpirable(ctx context.Context, key Key, expires bool) error
	Get(ctx context.Context, key Key) (Value, error)
}

// ProviderStore caches queries to IPNI
type ProviderStore Cache[mh.Multihash, []model.ProviderResult]

// ContentClaimsStore stores published content claims
type ContentClaimsStore Store[ipld.Link, delegation.Delegation]

// ContentClaimsCache caches fetched content claims
type ContentClaimsCache Cache[cid.Cid, delegation.Delegation]

// ShardedDagIndexStore caches fetched sharded dag indexes
type ShardedDagIndexStore Cache[EncodedContextID, blobindex.ShardedDagIndexView]

// Match narrows parameters for locating providers/claims for a set of multihashes
type Match struct {
	Subject []did.DID
}

// Query is a query for several multihashes
type Query struct {
	Hashes []mh.Multihash
	Match  Match
}

// QueryResult is an encodable result of a query
type QueryResult interface {
	ipld.View
	// Claims is a list of links to the root bock of claims that can be found in this message
	Claims() []ipld.Link
	// Indexes is a list of links to the CID hash of archived sharded dag indexes that can be found in this
	// message
	Indexes() []ipld.Link
}

type Getter interface {
	// Get retrieves a claim that has been published or cached by the
	// indexing service. No external sources are consulted.
	Get(ctx context.Context, claim ipld.Link) (delegation.Delegation, error)
}

type Publisher interface {
	// Cache caches a claim with the service temporarily.
	Cache(ctx context.Context, provider peer.AddrInfo, claim delegation.Delegation) error
	// Publish writes a claim to permanent storage, adds it to an IPNI
	// advertisement, annnounces it to IPNI nodes and caches it.
	Publish(ctx context.Context, claim delegation.Delegation) error
}

type Querier interface {
	// Query allows claims to be queried by their subject (content CID). It
	// returns claims as well as any relevant indexes.
	Query(ctx context.Context, q Query) (QueryResult, error)
}

// Service is the core methods of the indexing service.
type Service interface {
	Getter
	Publisher
	Querier
}
