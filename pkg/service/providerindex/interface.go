package providerindex

import (
	"context"
	"iter"

	"github.com/ipni/go-libipni/find/model"
	meta "github.com/ipni/go-libipni/metadata"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multihash"
)

// ProviderIndex is a read/write interface to a local cache of providers that falls back to legacy systems and IPNI
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
	// advertisement for them. If the expire arg is true entries expire after a
	// pre-determined time or not at all if it is false.
	Cache(ctx context.Context, provider peer.AddrInfo, contextID string, digests iter.Seq[multihash.Multihash], meta meta.Metadata, expire bool) error
	// CacheAsync writes entries to the cache asynchronously. It has mostly the
	// same semantics as [ProviderIndex.Cache] but since it is asynchronous it
	// does not guarantee entries to be present in the cache immediately after it
	// has returned.
	//
	// Asynchronously cached items are always expired after a pre-determined time,
	// this is because there are no guarantees for when the items will be written.
	CacheAsync(ctx context.Context, provider peer.AddrInfo, contextID string, digests iter.Seq[multihash.Multihash], meta meta.Metadata) error
	// Publish generates and stores an advertisement for the hashes and announces
	// it to IPNI.
	Publish(ctx context.Context, provider peer.AddrInfo, contextID string, digests iter.Seq[multihash.Multihash], meta meta.Metadata) error
}
