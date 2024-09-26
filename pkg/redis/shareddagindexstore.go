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

// ShardedDagIndexStore is a RedisStore for storing sharded dag indexes that implements types.ShardedDagIndexStore
type ShardedDagIndexStore = Store[types.EncodedContextID, blobindex.ShardedDagIndexView]

// NewShardedDagIndexStore returns a new instance of a ShardedDagIndex store using the given redis client
func NewShardedDagIndexStore(client Client) *ShardedDagIndexStore {
	return &Store[types.EncodedContextID, blobindex.ShardedDagIndexView]{shardedDagIndexFromRedis, shardedDagIndexToRedis, encodedContextIDKeyString, client}
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
