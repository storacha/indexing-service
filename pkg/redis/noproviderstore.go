package redis

import (
	// imported for embedding
	_ "embed"
	"encoding/binary"
	"errors"

	"github.com/multiformats/go-multicodec"
	multihash "github.com/multiformats/go-multihash"
	"github.com/storacha/indexing-service/pkg/types"
)

var (
	_ types.NoProviderStore = (*NoProviderStore)(nil)
)

var ErrDecodingMulticodec = errors.New("error parsing multicodec")

// NoProviderStore is a RedisStore for storing IPNI data that implements types.ProviderStore
type NoProviderStore = Store[multihash.Multihash, multicodec.Code]

// NewNoProviderStore returns a new instance of an IPNI store using the given redis client
func NewNoProviderStore(client Client, opts ...Option) *NoProviderStore {
	return NewStore(noProviderResultFromRedis, noProviderResultToRedis, multihashKeyString, client, opts...)
}

func noProviderResultFromRedis(data string) (multicodec.Code, error) {
	code, read := binary.Uvarint([]byte(data))
	if read <= 0 {
		return 0, ErrDecodingMulticodec
	}
	return multicodec.Code(code), nil
}

func noProviderResultToRedis(record multicodec.Code) (string, error) {
	buf := make([]byte, binary.MaxVarintLen64)
	binary.PutUvarint(buf, uint64(record))
	return string(buf), nil
}
