package redis

import (
	"io"

	cid "github.com/ipfs/go-cid"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/indexing-service/pkg/types"
)

var (
	_ types.ContentClaimsStore = (*ContentClaimsStore)(nil)
)

// ContentClaimsStore is a RedisStore for storing content claims that implements types.ContentClaimsStore
type ContentClaimsStore = Store[cid.Cid, delegation.Delegation]

// NewContentClaimsStore returns a new instance of a Content Claims Store using the given redis client
func NewContentClaimsStore(client Client) *ContentClaimsStore {
	return &Store[cid.Cid, delegation.Delegation]{delegationFromRedis, delegationToRedis, cidKeyString, client}
}

func delegationFromRedis(data string) (delegation.Delegation, error) {
	return delegation.Extract([]byte(data))
}

func delegationToRedis(d delegation.Delegation) (string, error) {
	r := delegation.Archive(d)
	data, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func cidKeyString(c cid.Cid) string {
	return multihashKeyString(c.Hash())
}
