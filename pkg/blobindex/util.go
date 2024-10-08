package blobindex

import (
	"bytes"

	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	mh "github.com/multiformats/go-multihash"
	"github.com/storacha/go-ucanto/core/car"
	"github.com/storacha/go-ucanto/core/ipld"
)

// FromShardArchives creates a sharded DAG index by indexing blocks in the passed CAR shards.
func FromShardArchives(content ipld.Link, shards [][]byte) (ShardedDagIndexView, error) {
	index := NewShardedDagIndexView(content, len(shards))
	for _, s := range shards {
		digest, err := mh.Sum(s, mh.SHA2_256, -1)
		if err != nil {
			return nil, err
		}
		_, blocks, err := car.Decode(bytes.NewReader(s))
		if err != nil {
			return nil, err
		}
		for blk, err := range blocks {
			if err != nil {
				return nil, err
			}
			cb := blk.(car.CarBlock)
			index.SetSlice(digest, blk.Link().(cidlink.Link).Cid.Hash(), Position{
				Offset: cb.Offset(),
				Length: cb.Length(),
			})
		}
	}
	return index, nil
}
