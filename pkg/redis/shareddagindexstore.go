package redis

import (
	"bytes"
	"io"

	"github.com/storacha-network/indexing-service/pkg/blobindex"
	"github.com/storacha-network/indexing-service/pkg/types"
)

var (
	_ types.ShardedDagIndexStore = (*ShardedDagIndexStore)(nil)
)

type ShardedDagIndexStore = RedisStore[types.EncodedContextID, blobindex.ShardedDagIndexView]

func NewShardedDagIndexStore(client RedisClient) *ShardedDagIndexStore {
	return &RedisStore[types.EncodedContextID, blobindex.ShardedDagIndexView]{shardedDagIndexFromRedis, shardedDagIndexToRedis, encodedContextIDKeyString, client}
}

func shardedDagIndexFromRedis(data string) (blobindex.ShardedDagIndexView, error) {
	return blobindex.Extract(bytes.NewReader([]byte(data)))
}

func shardedDagIndexToRedis(shardedDagIndex blobindex.ShardedDagIndexView) (string, error) {
	r, err := shardedDagIndex.Archive()
	if err != nil {
		return "", err
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func encodedContextIDKeyString(encodedContextID types.EncodedContextID) string {
	return string(encodedContextID)
}
