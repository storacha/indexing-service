package blobindex_test

import (
	"crypto/rand"
	"io"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime/datamodel"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	mh "github.com/multiformats/go-multihash"
	"github.com/storacha/go-ucanto/core/car"
	"github.com/storacha/go-ucanto/core/ipld/block"
	"github.com/storacha/indexing-service/pkg/blobindex"
	"github.com/stretchr/testify/require"
)

func TestFromToArchive(t *testing.T) {
	roots, contentCar := randomCAR(32)
	contentCarBytes, _ := io.ReadAll(contentCar)
	index, err := blobindex.FromShardArchives(roots[0], [][]byte{contentCarBytes})
	require.NoError(t, err)
	r, err := index.Archive()
	require.NoError(t, err)
	newIndex, err := blobindex.Extract(r)
	require.NoError(t, err)
	require.NotZero(t, newIndex.Shards().Size())
	require.Equal(t, index.Shards().Size(), newIndex.Shards().Size())
	for key, shard := range newIndex.Shards().Iterator() {
		require.True(t, index.Shards().Has(key))
		require.Equal(t, index.Shards().Get(key).Size(), shard.Size())
	}
}

func randomBytes(size int) []byte {
	bytes := make([]byte, size)
	_, _ = rand.Reader.Read(bytes)
	return bytes
}

func randomCAR(size int) ([]datamodel.Link, io.Reader) {
	bytes := randomBytes(size)
	c, _ := cid.Prefix{
		Version:  1,
		Codec:    cid.Raw,
		MhType:   mh.SHA2_256,
		MhLength: -1,
	}.Sum(bytes)

	link := cidlink.Link{Cid: c}
	r := car.Encode([]datamodel.Link{link}, func(yield func(block.Block, error) bool) {
		yield(block.NewBlock(link, bytes), nil)
	})
	return []datamodel.Link{link}, r
}

func randomCID() datamodel.Link {
	bytes := randomBytes(10)
	c, _ := cid.Prefix{
		Version:  1,
		Codec:    cid.Raw,
		MhType:   mh.SHA2_256,
		MhLength: -1,
	}.Sum(bytes)
	return cidlink.Link{Cid: c}
}
