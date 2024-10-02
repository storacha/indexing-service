package blobindex

import (
	mh "github.com/multiformats/go-multihash"
	"github.com/storacha-network/indexing-service/pkg/internal/bytemap"
)

// NewMultihashMap returns a new map of multihash to a data type
func NewMultihashMap[T any](sizeHint int) MultihashMap[T] {
	return bytemap.NewByteMap[mh.Multihash, T](sizeHint)
}
