package redis_test

import (
	"context"
	"testing"

	"github.com/storacha/indexing-service/pkg/internal/testutil"
	"github.com/storacha/indexing-service/pkg/redis"
	"github.com/storacha/indexing-service/pkg/types"
	"github.com/stretchr/testify/require"
)

func TestShardedDagIndexStore(t *testing.T) {
	mockRedis := NewMockRedis()
	shardedDagIndexStore := redis.NewShardedDagIndexStore(mockRedis)
	root1, index1 := testutil.RandomShardedDagIndexView(32)
	root2, index2 := testutil.RandomShardedDagIndexView(32)

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
