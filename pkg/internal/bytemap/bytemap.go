package bytemap

import (
	"iter"
	"maps"

	"github.com/storacha/go-ucanto/core/iterable"
)

type byteMap[K ~[]byte, T any] struct {
	data map[string]T
}

// ByteMap is a generic for mapping byte array like types to arbitrary data types
type ByteMap[K ~[]byte, T any] interface {
	Get(K) T
	Has(K) bool
	Set(K, T)
	Delete(K) bool
	Size() int
	Iterator() iter.Seq2[K, T]
	Keys() iter.Seq[K]
	Values() iter.Seq[T]
}

// NewByteMap returns a new map of multihash to a data type
func NewByteMap[K ~[]byte, T any](sizeHint int) ByteMap[K, T] {
	var stringMap map[string]T
	if sizeHint == -1 {
		stringMap = make(map[string]T)
	} else {
		stringMap = make(map[string]T, sizeHint)
	}
	return &byteMap[K, T]{stringMap}
}

func (bm *byteMap[K, T]) Get(b K) T {
	return bm.data[string(b)]
}

func (bm *byteMap[K, T]) Has(b K) bool {
	_, ok := bm.data[string(b)]
	return ok
}

func (bm *byteMap[K, T]) Set(b K, t T) {
	bm.data[string(b)] = t
}

func (bm *byteMap[K, T]) Delete(b K) bool {
	_, ok := bm.data[string(b)]
	delete(bm.data, string(b))
	return ok
}

func (bm *byteMap[K, T]) Size() int {
	return len(bm.data)
}

func (bm *byteMap[K, T]) Iterator() iter.Seq2[K, T] {
	return iterable.Map2(func(str string, t T) (K, T) {
		return K(str), t
	}, maps.All(bm.data))
}

func (bm *byteMap[K, T]) Keys() iter.Seq[K] {
	return iterable.Map(func(str string) K {
		return K(str)
	}, maps.Keys(bm.data))
}

func (bm *byteMap[K, T]) Values() iter.Seq[T] {
	return maps.Values(bm.data)
}
