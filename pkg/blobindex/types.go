package blobindex

import (
	"io"
	"iter"

	mh "github.com/multiformats/go-multihash"
	"github.com/storacha-network/go-ucanto/core/ipld"
	dm "github.com/storacha-network/indexing-service/pkg/blobindex/datamodel"
)

// MultihashMap is a generic for mapping multihashes to arbitrary data types
type MultihashMap[T any] interface {
	Get(mh.Multihash) T
	Has(mh.Multihash) bool
	Set(mh.Multihash, T)
	Delete(mh.Multihash) bool
	Size() int
	Iterator() iter.Seq2[mh.Multihash, T]
}

// Position describes an offet and length within a shard
type Position = dm.PositionModel

// ShardedDagIndex descriptes a DAG stored over one or multiple shards
type ShardedDagIndex interface {
	/** DAG root CID that the index pertains to. */
	Content() ipld.Link
	/** Index information for shards the DAG is split across. */
	Shards() MultihashMap[MultihashMap[Position]]
}

// ShardedDagIndexView is an interface for building shard DAG indexes
// and writing them to CAR files
type ShardedDagIndexView interface {
	ShardedDagIndex
	SetSlice(shard mh.Multihash, slice mh.Multihash, position Position)
	Archive() (io.Reader, error)
}
