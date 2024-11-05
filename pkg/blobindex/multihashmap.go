package blobindex

import (
	mh "github.com/multiformats/go-multihash"
	"github.com/storacha/indexing-service/pkg/bytemap"
)

// NewMultihashMap returns a new map of multihash to a data type
func NewMultihashMap[T any](sizeHint int) MultihashMap[T] {
	return bytemap.NewByteMap[mh.Multihash, T](sizeHint)
}
