package provider

import (
	"bytes"

	"github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"
	"github.com/web3-storage/go-ucanto/did"
)

func (c ContextID) ToKey() ([]byte, error) {
	return mh.Sum(bytes.Join([][]byte{c.Space.Bytes(), c.Cid.Bytes()}, nil), mh.SHA2_256, -1)
}

type ContextID struct {
	Space did.DID
	Cid   cid.Cid
}

type IPNIQueryKey struct {
	Space *did.DID
	Hash  mh.Multihash
}
