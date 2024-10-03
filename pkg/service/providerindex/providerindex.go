package providerindex

import (
	"bytes"
	"context"
	"slices"

	"github.com/ipld/go-ipld-prime"
	"github.com/ipni/go-libipni/announce"
	"github.com/ipni/go-libipni/dagsync"
	ipnifind "github.com/ipni/go-libipni/find/client"
	"github.com/ipni/go-libipni/find/model"
	"github.com/multiformats/go-multicodec"
	mh "github.com/multiformats/go-multihash"
	"github.com/storacha/go-ucanto/did"
	"github.com/storacha/indexing-service/pkg/metadata"
	"github.com/storacha/indexing-service/pkg/types"
)

type QueryKey struct {
	Spaces       []did.DID
	Hash         mh.Multihash
	TargetClaims []multicodec.Code
}

// ProviderIndex is a read/write interface to a local cache of providers that falls back to IPNI
type ProviderIndex struct {
	providerStore types.ProviderStore
	findClient    ipnifind.Client
}

// TBD access to legacy systems
type LegacySystems interface{}

// TODO: This assumes using low level primitives for publishing from IPNI but maybe we want to go ahead and use index-provider?
func NewProviderIndex(ipniCache types.ProviderStore, findClient ipnifind.Client, sender announce.Sender, publisher dagsync.Publisher, advertisementsLsys ipld.LinkSystem, legacySystems LegacySystems) *ProviderIndex {
	return &ProviderIndex{
		providerStore: ipniCache,
		findClient:    findClient,
	}
}

// Find should do the following
//  1. Read from the IPNI Storage cache to get a list of providers
//     a. If there is no record in cache, query IPNI, filter out any non-content claims metadata, and store
//     the resulting records in the cache
//     b. the are no records in the cache or IPNI, it can attempt to read from legacy systems -- Dynamo tables & content claims storage, synthetically constructing provider results
//  2. With returned provider results, filter additionally for claim type. If space dids are set, calculate an encodedcontextid's by hashing space DID and Hash, and filter for a matching context id
//     Future TODO: kick off a conversion task to update the recrds
func (pi *ProviderIndex) Find(ctx context.Context, qk QueryKey) ([]model.ProviderResult, error) {
	results, err := pi.getProviderResults(ctx, qk.Hash)
	if err != nil {
		return nil, err
	}
	results, err = pi.filteredCodecs(results, qk.TargetClaims)
	if err != nil {
		return nil, err
	}
	return pi.filterBySpace(results, qk.Hash, qk.Spaces)
}

func (pi *ProviderIndex) getProviderResults(ctx context.Context, mh mh.Multihash) ([]model.ProviderResult, error) {
	res, err := pi.providerStore.Get(ctx, mh)
	if err == nil {
		return res, nil
	}
	if err != types.ErrKeyNotFound {
		return nil, err
	}

	findRes, err := pi.findClient.Find(ctx, mh)
	if err != nil {
		return nil, err
	}
	var results []model.ProviderResult
	for _, mhres := range findRes.MultihashResults {
		results = append(results, mhres.ProviderResults...)
	}
	err = pi.providerStore.Set(ctx, mh, results, true)
	if err != nil {
		return nil, err
	}
	return results, nil
}

func (pi *ProviderIndex) filteredCodecs(results []model.ProviderResult, codecs []multicodec.Code) ([]model.ProviderResult, error) {
	if len(codecs) == 0 {
		return results, nil
	}
	return filter(results, func(result model.ProviderResult) (bool, error) {
		md := metadata.MetadataContext.New()
		err := md.UnmarshalBinary(result.Metadata)
		if err != nil {
			return false, err
		}
		return slices.ContainsFunc(codecs, func(code multicodec.Code) bool {
			return slices.ContainsFunc(md.Protocols(), func(mdCode multicodec.Code) bool {
				return mdCode == code
			})
		}), nil
	})
}

func (pi *ProviderIndex) filterBySpace(results []model.ProviderResult, mh mh.Multihash, spaces []did.DID) ([]model.ProviderResult, error) {
	if len(spaces) == 0 {
		return results, nil
	}
	encryptedIds := make([]types.EncodedContextID, 0, len(spaces))
	for _, space := range spaces {
		encryptedId, err := types.ContextID{
			Space: &space,
			Hash:  mh,
		}.ToEncoded()
		if err != nil {
			return nil, err
		}
		encryptedIds = append(encryptedIds, encryptedId)
	}

	filtered, err := filter(results, func(result model.ProviderResult) (bool, error) {
		return slices.ContainsFunc(encryptedIds, func(encyptedID types.EncodedContextID) bool {
			return bytes.Equal(result.ContextID, encyptedID)
		}), nil
	})
	if err != nil {
		return nil, err
	}
	if len(filtered) > 0 {
		return filtered, nil
	}
	return results, nil
}

// Publish should do the following:
// 1. Write the entries to the cache with no expiration until publishing is complete
// 2. Generate an advertisement for the advertised hashes and publish/announce it
func (pi *ProviderIndex) Publish(context.Context, []mh.Multihash, model.ProviderResult) {

}

func filter(results []model.ProviderResult, filterFunc func(model.ProviderResult) (bool, error)) ([]model.ProviderResult, error) {

	filtered := make([]model.ProviderResult, 0, len(results))
	for _, result := range results {
		include, err := filterFunc(result)
		if err != nil {
			return nil, err
		}
		if include {
			filtered = append(filtered, result)
		}
	}
	return filtered, nil
}
