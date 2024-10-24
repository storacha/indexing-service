package providerindex

import (
	"context"

	"github.com/ipni/go-libipni/find/model"
	meta "github.com/ipni/go-libipni/metadata"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multihash"
)

// ProviderIndex is a read/write interface to a local cache of providers that falls back to IPNI
type ProviderIndex interface {
	// Find should do the following
	//  1. Read from the IPNI Storage cache to get a list of providers
	//     a. If there is no record in cache, query IPNI, filter out any non-content claims metadata, and store
	//     the resulting records in the cache
	//     b. the are no records in the cache or IPNI, it can attempt to read from legacy systems -- Dynamo tables & content claims storage, synthetically constructing provider results
	//  2. With returned provider results, filter additionally for claim type. If space dids are set, calculate an encodedcontextid's by hashing space DID and Hash, and filter for a matching context id
	//     Future TODO: kick off a conversion task to update the recrds
	Find(context.Context, QueryKey) ([]model.ProviderResult, error)
	// Cache writes entries to the cache but does not publish/announce an
	// advertisement for them. Entries expire after a pre-determined time.
	Cache(ctx context.Context, provider peer.AddrInfo, contextID string, digests []multihash.Multihash, meta meta.Metadata) error
	// Publish should do the following:
	// 1. Write the entries to the cache with no expiration until publishing is complete
	// 2. Generate an advertisement for the advertised hashes and publish/announce it
	Publish(ctx context.Context, provider peer.AddrInfo, contextID string, digests []multihash.Multihash, meta meta.Metadata) error
}
