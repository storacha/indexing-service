package redis

import (
	// imported for embedding
	_ "embed"

	"github.com/ipni/go-libipni/find/model"
	multihash "github.com/multiformats/go-multihash"
	"github.com/storacha/indexing-service/pkg/providerresults"
	"github.com/storacha/indexing-service/pkg/types"
)

var (
	_ types.ProviderStore = (*ProviderStore)(nil)
)

// ProviderStore is a RedisStore for storing IPNI data that implements types.ProviderStore
type ProviderStore = SetsStore[multihash.Multihash, model.ProviderResult]

// NewProviderStore returns a new instance of an IPNI store using the given redis client
func NewProviderStore(client SetsClient) *ProviderStore {
	return NewSetsStore(providerResultFromRedis, providerResultToRedis, multihashKeyString, client)
}

func providerResultFromRedis(data string) (model.ProviderResult, error) {
	return providerresults.UnmarshalCBOR([]byte(data))
}

func providerResultToRedis(record model.ProviderResult) (string, error) {
	data, err := providerresults.MarshalCBOR(record)
	return string(data), err
}

func multihashKeyString(k multihash.Multihash) string {
	return string(k)
}
