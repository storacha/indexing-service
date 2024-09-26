package redis

import (
	"io"

	cid "github.com/ipfs/go-cid"
	"github.com/storacha-network/go-ucanto/core/delegation"
	"github.com/storacha-network/indexing-service/pkg/types"
)

var (
	_ types.ContentClaimsStore = (*ContentClaimsStore)(nil)
)

type ContentClaimsStore = RedisStore[cid.Cid, delegation.Delegation]

func NewContentClaimsStore(client RedisClient) *ContentClaimsStore {
	return &RedisStore[cid.Cid, delegation.Delegation]{delegationFromRedis, delegationToRedis, cidKeyString, client}
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
