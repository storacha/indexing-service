package blobindex

import (
	"iter"
	"maps"

	mh "github.com/multiformats/go-multihash"
	"github.com/storacha-network/go-ucanto/core/iterable"
)

type multihashMap[T any] struct {
	data map[string]T
}

// NewMultihashMap returns a new map of multihash to a data type
func NewMultihashMap[T any](sizeHint int) MultihashMap[T] {
	var stringMap map[string]T
	if sizeHint == -1 {
		stringMap = make(map[string]T)
	} else {
		stringMap = make(map[string]T, sizeHint)
	}
	return &multihashMap[T]{stringMap}
}

func (mhm *multihashMap[T]) Get(mh mh.Multihash) T {
	return mhm.data[string(mh)]
}

func (mhm *multihashMap[T]) Has(mh mh.Multihash) bool {
	_, ok := mhm.data[string(mh)]
	return ok
}

func (mhm *multihashMap[T]) Set(mh mh.Multihash, t T) {
	mhm.data[string(mh)] = t
}

func (mhm *multihashMap[T]) Delete(mh mh.Multihash) bool {
	_, ok := mhm.data[string(mh)]
	delete(mhm.data, string(mh))
	return ok
}

func (mhm *multihashMap[T]) Size() int {
	return len(mhm.data)
}
func (mhm *multihashMap[T]) Iterator() iter.Seq2[mh.Multihash, T] {
	return iterable.Map2(func(str string, t T) (mh.Multihash, T) {
		return mh.Multihash(str), t
	}, maps.All(mhm.data))
}
