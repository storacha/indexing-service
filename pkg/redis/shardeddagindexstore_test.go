package redis_test

import (
	"context"
	"io"
	"testing"

	cid "github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"
	"github.com/storacha/indexing-service/pkg/blobindex"
	"github.com/storacha/indexing-service/pkg/internal/testutil"
	"github.com/storacha/indexing-service/pkg/redis"
	"github.com/storacha/indexing-service/pkg/types"
	"github.com/stretchr/testify/require"
)

func TestShardedDagIndexStore(t *testing.T) {
	mockRedis := NewMockRedis()
	shardedDagIndexStore := redis.NewShardedDagIndexStore(mockRedis)
	root1, index1 := testutil.Must2(randomShardedDagIndexView(32))(t)
	root2, index2 := testutil.Must2(randomShardedDagIndexView(32))(t)

	aliceDid := testutil.Alice.DID()
	encodedID1 := testutil.Must(types.ContextID{
		Space: &aliceDid,
		Hash:  root1.Hash(),
	}.ToEncoded())(t)
	encodedID2 := testutil.Must(types.ContextID{
		Hash: root2.Hash(),
	}.ToEncoded())(t)

	ctx := context.Background()
	require.NoError(t, shardedDagIndexStore.Set(ctx, encodedID1, index1, false))
	require.NoError(t, shardedDagIndexStore.Set(ctx, encodedID2, index2, true))

	returnedIndex1 := testutil.Must(shardedDagIndexStore.Get(ctx, encodedID1))(t)
	returnedIndex2 := testutil.Must(shardedDagIndexStore.Get(ctx, encodedID2))(t)
	testutil.RequireEqualIndex(t, index1, returnedIndex1)
	testutil.RequireEqualIndex(t, index2, returnedIndex2)
}

func randomShardedDagIndexView(size int) (cid.Cid, blobindex.ShardedDagIndexView, error) {
	roots, contentCar := testutil.RandomCAR(size)
	contentCarBytes, err := io.ReadAll(contentCar)
	if err != nil {
		return cid.Undef, nil, err
	}

	root, err := cid.Prefix{
		Version:  1,
		Codec:    cid.Raw,
		MhType:   mh.SHA2_256,
		MhLength: -1,
	}.Sum(contentCarBytes)

	if err != nil {
		return cid.Undef, nil, err
	}

	shard, err := blobindex.FromShardArchives(roots[0], [][]byte{contentCarBytes})
	return root, shard, err
}
