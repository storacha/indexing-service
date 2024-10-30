package digestutil

import (
	"fmt"

	"github.com/multiformats/go-multibase"
	"github.com/multiformats/go-multihash"
)

func Format(digest multihash.Multihash) string {
	key, _ := multibase.Encode(multibase.Base58BTC, digest)
	return key
}

func Parse(input string) (multihash.Multihash, error) {
	_, bytes, err := multibase.Decode(input)
	if err != nil {
		return nil, fmt.Errorf("decoding multibase encoded digest: %s", err)
	}
	digest, err := multihash.Cast(bytes)
	if err != nil {
		return nil, fmt.Errorf("invalid multihash digest: %s", err)
	}
	return digest, nil
}
