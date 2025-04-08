package datamodeltype

import (
	// for go:embed directive
	_ "embed"
	"fmt"

	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/schema"
)

//go:embed shardeddagindex.ipldsch
var shardedDagIndex []byte

var shardedTs *schema.TypeSystem

func init() {
	ts, err := ipld.LoadSchemaBytes(shardedDagIndex)
	if err != nil {
		panic(fmt.Errorf("loading sharded dag index schema: %w", err))
	}
	shardedTs = ts
}

// ShardedDagIndexSchema returns the schema type for a sharded dag index
func ShardedDagIndexSchema() schema.Type {
	return shardedTs.TypeByName("ShardedDagIndex")
}

// BlobIndexSchema returns the schema type for a blob index of a single shard
func BlobIndexSchema() schema.Type {
	return shardedTs.TypeByName("BlobIndex")
}

// ShardedDagIndexModel is the golang structure for encoding sharded DAG index header blocks
type ShardedDagIndexModel struct {
	DagO_1 *ShardedDagIndexModel_0_1
}

// ShardedDagIndexModel_0_1 describes the 0.1 version of ShardedDagIndex
type ShardedDagIndexModel_0_1 struct {
	Content ipld.Link
	Shards  []ipld.Link
}

// PositionModel is an offset and length for a since CID in a blob
type PositionModel struct {
	Offset uint64
	Length uint64
}

// BlobSliceModel describes a multihash and its position in a blob
type BlobSliceModel struct {
	Multihash []byte
	Position  PositionModel
}

// BlobIndexModel is the golang structure for encoding a shard of CIDS in a block
type BlobIndexModel struct {
	Multihash []byte
	Slices    []BlobSliceModel
}
