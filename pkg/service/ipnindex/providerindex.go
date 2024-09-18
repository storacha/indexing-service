package provider

import (
	"github.com/ipld/go-ipld-prime"
	"github.com/ipni/go-libipni/announce"
	"github.com/ipni/go-libipni/dagsync"
	ipnifind "github.com/ipni/go-libipni/find/client"
	"github.com/ipni/go-libipni/find/model"
	mh "github.com/multiformats/go-multihash"
	"github.com/storacha-network/go-ucanto/did"
	"github.com/storacha-network/indexing-service/pkg/metadata"
	"github.com/storacha-network/indexing-service/pkg/types"
)

type QueryKey struct {
	Spaces    []did.DID
	Hash      mh.Multihash
	ClaimType metadata.ClaimType
}

// IPNIIndex is a read/write interface to a local cache that falls back to IPNI, exclusively publishing claims data
type IPNIIndex interface {
	// Find should do the following
	//  1. Read from the IPNI Storage cache to get a list of providers
	//     a. If there is no record in cache, query IPNI, filter out any non-content claims metadata, and store
	//     the resulting records in the cache
	//     b. the are no records in the cache or IPNI, it can attempt to read from legacy systems -- Dynamo tables & content claims storage, synthetically constructing provider results
	//  2. With returned provider results, filter additionally for claim type. If space dids are set, calculate an encodedcontextid's by hashing space DID and Hash, and filter for a matching context id
	//     Future TODO: kick off a conversion task to update the recrds
	Find(QueryKey) []model.ProviderResult
	// Publish should do the following:
	// 1. Write the entries to the cache with no expiration until publishing is complete
	// 2. Generate an advertisement for the advertised hashes and publish/announce it
	Publish([]mh.Multihash, model.ProviderResult)
}

// TBD access to legacy systems
type LegacySystems interface{}

// TODO: This assumes using low level primitives for publishing from IPNI but maybe we want to go ahead and use index-provider?
func NewIPNIIndex(ipniCache types.IPNIStore, findClient ipnifind.Client, sender announce.Sender, publisher dagsync.Publisher, advertisementsLsys ipld.LinkSystem, legacySystems LegacySystems) IPNIIndex {
	return nil
}
