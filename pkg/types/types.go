package types

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/ipfs/go-cid"
	"github.com/ipni/go-libipni/find/model"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multicodec"
	mh "github.com/multiformats/go-multihash"
	"github.com/storacha/go-libstoracha/blobindex"
	"github.com/storacha/go-libstoracha/metadata"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/go-ucanto/core/ipld"
	"github.com/storacha/go-ucanto/did"
	"github.com/storacha/go-ucanto/ucan"
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

// ValueSetCache describes a cache interface whose values are sets
type ValueSetCache[Key, Value any] interface {
	Add(ctx context.Context, key Key, values ...Value) (uint64, error)
	SetExpirable(ctx context.Context, key Key, expires bool) error
	Members(ctx context.Context, key Key) ([]Value, error)
}

// BatchingValueSetCache is a value-set cache that can batch updates.
// Note: a batch is not a transaction.
type BatchingValueSetCache[Key, Value any] interface {
	ValueSetCache[Key, Value]
	Batch() ValueSetCacheBatcher[Key, Value]
}

type ValueSetCacheBatcher[Key, Value any] interface {
	Add(ctx context.Context, key Key, values ...Value) error
	SetExpirable(ctx context.Context, key Key, expires bool) error
	Commit(ctx context.Context) error
}

// ProviderStore caches queries to IPNI
type ProviderStore BatchingValueSetCache[mh.Multihash, model.ProviderResult]

// NoProviderStore caches which queries for providers returned no results
type NoProviderStore ValueSetCache[mh.Multihash, multicodec.Code]

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

// QueryType allows defining which claims a query is targeting. QueryTypeStandard targets all claims,
// i.e. Location, Index and Equals
type QueryType int

const (
	QueryTypeStandard QueryType = iota
	QueryTypeLocation
	QueryTypeIndexOrLocation
	QueryTypeStandardCompressed
	QueryTypeEquals
)

func (qt QueryType) String() string {
	switch qt {
	case QueryTypeStandard:
		return "standard"
	case QueryTypeLocation:
		return "location"
	case QueryTypeIndexOrLocation:
		return "index_or_location"
	case QueryTypeStandardCompressed:
		return "standard_compressed"
	case QueryTypeEquals:
		return "equals"
	default:
		return "invalid"
	}
}

func ParseQueryType(queryTypeStr string) (QueryType, error) {
	switch queryTypeStr {
	case QueryTypeStandard.String():
		return QueryTypeStandard, nil
	case QueryTypeLocation.String():
		return QueryTypeLocation, nil
	case QueryTypeIndexOrLocation.String():
		return QueryTypeIndexOrLocation, nil
	case QueryTypeStandardCompressed.String():
		return QueryTypeStandardCompressed, nil
	default:
		return 0, fmt.Errorf("invalid query type: %s", queryTypeStr)
	}
}

// Query is a query for several multihashes
type Query struct {
	Type   QueryType
	Hashes []mh.Multihash
	Match  Match
	// Delegations allowing the indexer to retrieve bytes from the network. These
	// are typically `space/content/retrieve` delegations for each subject (space)
	// in the [Match] paremeter.
	//
	// Delegations are sent in the `X-Agent-Message` HTTP header and MUST NOT
	// exceed 4kb in size.
	Delegations []delegation.Delegation
}

// QueryResult is an encodable result of a query
type QueryResult interface {
	ipld.View
	// Claims is a list of links to the root block of claims that can be found in this message
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

// RetrievalRequest is all the details needed for retrieving data from the
// network. At minimum it contains the URL to retrieve a blob from.
//
// Legacy retrievals will not carry any retrieval authorization information i.e.
// the Auth field will be nil.
type RetrievalRequest struct {
	// URL where the blob may be requested from.
	URL *url.URL
	// Optional byte range to request.
	Range *metadata.Range
	// Optional UCAN authorization parameters.
	Auth *RetrievalAuth
}

// NewRetrievalRequest creates a new [RetrievalRequest] object that contains all the
// details required to retrieve a blob from the network.
func NewRetrievalRequest(
	url *url.URL,
	byteRange *metadata.Range,
	auth *RetrievalAuth,
) RetrievalRequest {
	return RetrievalRequest{url, byteRange, auth}
}

// RetrievalAuth are the details for a UCAN authorized content retrieval.
//
// The provided proofs are expected to contain the `space/content/retrieve`
// delegated capability allowing content to be retrieved using UCAN
// authorization.
type RetrievalAuth struct {
	// The Indexing Service UCAN signing key.
	Issuer ucan.Signer
	// Identity of the storage node to retrieve data from.
	Audience ucan.Principal
	// Retrieval ability, resource (typically the space) and caveats.
	Capability ucan.Capability[ucan.CaveatBuilder]
	// Delegations from the client (e.g. `space/content/retrieve`) or a storage
	// node (e.g. `blob/retrieve`) to the indexing service authorizing retrieval.
	Proofs []delegation.Proof
}

// NewRetrievalAuth creates a new [RetrievalAuth] object for UCAN authorizing
// blob retrievals.
func NewRetrievalAuth[C ucan.CaveatBuilder](
	issuer ucan.Signer,
	audience ucan.Principal,
	capability ucan.Capability[C],
	proofs []delegation.Proof,
) RetrievalAuth {
	return RetrievalAuth{
		issuer,
		audience,
		ucan.NewCapability[ucan.CaveatBuilder](
			capability.Can(),
			capability.With(),
			capability.Nb(),
		),
		proofs,
	}
}
