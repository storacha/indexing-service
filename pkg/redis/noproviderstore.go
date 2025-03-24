package redis

import (
	// imported for embedding
	_ "embed"

	multihash "github.com/multiformats/go-multihash"
	"github.com/storacha/indexing-service/pkg/types"
)

var (
	_ types.NoProviderStore = (*NoProviderStore)(nil)
)

// NoProviderStore is a RedisStore for storing IPNI data that implements types.ProviderStore
type NoProviderStore = Store[multihash.Multihash, struct{}]

// NewNoProviderStore returns a new instance of an IPNI store using the given redis client
func NewNoProviderStore(client Client, opts ...Option) *NoProviderStore {
	return NewStore(noProviderResultFromRedis, noProviderResultToRedis, multihashKeyString, client, opts...)
}

func noProviderResultFromRedis(data string) (struct{}, error) {
	return struct{}{}, nil
}

func noProviderResultToRedis(record struct{}) (string, error) {
	return "", nil
}
