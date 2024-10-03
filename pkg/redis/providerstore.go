package redis

import (
	// imported for embedding
	_ "embed"
	"fmt"

	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/codec/dagcbor"
	"github.com/ipld/go-ipld-prime/node/bindnode"
	"github.com/ipld/go-ipld-prime/schema"
	"github.com/ipni/go-libipni/find/model"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	multihash "github.com/multiformats/go-multihash"
	"github.com/storacha-network/indexing-service/pkg/types"
)

var (
	//go:embed providerresult.ipldsch
	providerResultsBytes []byte
	peerIDConverter      = bindnode.NamedBytesConverter("PeerID", bytesToPeerID, peerIDtoBytes)
	multiaddrConverter   = bindnode.NamedBytesConverter("Multiaddr", bytesToMultiaddr, multiaddrToBytes)
	providerResultsType  schema.Type
	_                    types.ProviderStore = (*ProviderStore)(nil)
)

func init() {
	typeSystem, err := ipld.LoadSchemaBytes(providerResultsBytes)
	if err != nil {
		panic(fmt.Errorf("failed to load schema: %w", err))
	}
	providerResultsType = typeSystem.TypeByName("ProviderResults")
}

// ProviderStore is a RedisStore for storing IPNI data that implements types.ProviderStore
type ProviderStore = Store[multihash.Multihash, []model.ProviderResult]

// NewProviderStore returns a new instance of an IPNI store using the given redis client
func NewProviderStore(client Client) *ProviderStore {
	return NewStore(providerResultsFromRedis, providerResultsToRedis, multihashKeyString, client)
}

func bytesToPeerID(data []byte) (interface{}, error) {
	id, err := peer.IDFromBytes(data)
	return &id, err
}

func peerIDtoBytes(peerID interface{}) ([]byte, error) {
	return []byte(*peerID.(*peer.ID)), nil
}

func bytesToMultiaddr(data []byte) (interface{}, error) {
	ma, err := multiaddr.NewMultiaddrBytes(data)
	return &ma, err
}

func multiaddrToBytes(ma interface{}) ([]byte, error) {
	return (*ma.(*multiaddr.Multiaddr)).Bytes(), nil
}

func providerResultsFromRedis(data string) ([]model.ProviderResult, error) {
	var records []model.ProviderResult
	_, err := ipld.Unmarshal([]byte(data), dagcbor.Decode, &records, providerResultsType, peerIDConverter, multiaddrConverter)
	if err != nil {
		return nil, err
	}
	return records, nil
}

func providerResultsToRedis(records []model.ProviderResult) (string, error) {
	data, err := ipld.Marshal(dagcbor.Encode, &records, providerResultsType, peerIDConverter, multiaddrConverter)
	return string(data), err
}

func multihashKeyString(k multihash.Multihash) string {
	return string(k)
}
