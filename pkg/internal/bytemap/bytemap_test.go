package bytemap

import (
	"testing"

	"github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/require"
)

func TestKeys(t *testing.T) {
	bm := NewByteMap[multihash.Multihash, struct{}](-1)

	d0, err := multihash.Sum([]byte{1, 2, 3}, multihash.SHA2_256, -1)
	require.NoError(t, err)
	d1, err := multihash.Sum([]byte{4, 5, 6}, multihash.SHA2_256, -1)
	require.NoError(t, err)
	d2, err := multihash.Sum([]byte{7, 8, 9}, multihash.SHA2_256, -1)
	require.NoError(t, err)

	digests := []multihash.Multihash{d0, d1, d2}
	for _, d := range digests {
		bm.Set(d, struct{}{})
	}

	var keys []multihash.Multihash
	for d := range bm.Keys() {
		keys = append(keys, d)
	}

	require.ElementsMatch(t, digests, keys)
}

func TestValues(t *testing.T) {
	bm := NewByteMap[multihash.Multihash, string](-1)

	strings := []string{"this", "is", "fine"}
	for _, v := range strings {
		bm.Set([]byte("key"+v), v)
	}

	var values []string
	for d := range bm.Values() {
		values = append(values, d)
	}

	require.ElementsMatch(t, strings, values)
}
