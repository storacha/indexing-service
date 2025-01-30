package providerindex

import (
	"bytes"
	"context"
	"fmt"
	"iter"
	"slices"

	logging "github.com/ipfs/go-log/v2"
	ipnifind "github.com/ipni/go-libipni/find/client"
	"github.com/ipni/go-libipni/find/model"
	meta "github.com/ipni/go-libipni/metadata"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multicodec"
	mh "github.com/multiformats/go-multihash"
	"github.com/storacha/go-metadata"
	"github.com/storacha/go-ucanto/did"
	"github.com/storacha/indexing-service/pkg/internal/jobqueue"
	"github.com/storacha/indexing-service/pkg/types"
	"github.com/storacha/ipni-publisher/pkg/publisher"
)

var log = logging.Logger("providerindex")

type QueryKey struct {
	Spaces       []did.DID
	Hash         mh.Multihash
	TargetClaims []multicodec.Code
}

// ProviderIndexService is a read/write interface to a local cache of providers that falls back to IPNI
type ProviderIndexService struct {
	providerStore types.ProviderStore
	findClient    ipnifind.Finder
	publisher     publisher.Publisher
	legacyClaims  LegacyClaimsFinder
}

var _ ProviderIndex = (*ProviderIndexService)(nil)

func New(providerStore types.ProviderStore, findClient ipnifind.Finder, publisher publisher.Publisher, legacyClaims LegacyClaimsFinder) *ProviderIndexService {
	return &ProviderIndexService{
		providerStore: providerStore,
		findClient:    findClient,
		publisher:     publisher,
		legacyClaims:  legacyClaims,
	}
}

// Find should do the following
//  1. Read from the IPNI Storage cache to get a list of providers
//     a. if there are no records in the cache, query IPNI, filtering out any non-content claims metadata
//     b. if no records are found in IPNI, attempt to read claims from legacy systems -- Dynamo tables & content
//     claims storage, synthetically constructing provider results
//     c. finally, store the resulting records in the cache
//  2. With returned provider results, filter additionally for claim type. If space dids are set, calculate an
//     encodedcontextid's by hashing space DID and Hash, and filter for a matching context id
//     Future TODO: kick off a conversion task to update the records
func (pi *ProviderIndexService) Find(ctx context.Context, qk QueryKey) ([]model.ProviderResult, error) {
	results, err := pi.getProviderResults(ctx, qk.Hash, qk.TargetClaims)
	if err != nil {
		return nil, err
	}

	return filterBySpace(results, qk.Hash, qk.Spaces)
}

func (pi *ProviderIndexService) getProviderResults(ctx context.Context, mh mh.Multihash, targetClaims []multicodec.Code) ([]model.ProviderResult, error) {
	res, err := pi.providerStore.Members(ctx, mh)
	if err == nil {
		return res, nil
	}
	if err != types.ErrKeyNotFound {
		return nil, err
	}

	results, err := pi.fetchFromIPNI(ctx, mh, targetClaims)
	if err != nil {
		return nil, err
	}

	// if nothing was found on IPNI, try legacy claims storage
	if len(results) == 0 {
		legacyResults, err := pi.fetchFromLegacy(ctx, mh, targetClaims)
		if err != nil {
			return nil, err
		}

		results = legacyResults
	}

	// cache results if there are results to cache
	if len(results) > 0 {
		n, err := pi.providerStore.Add(ctx, mh, results...)
		if err != nil {
			return nil, err
		}
		if n > 0 {
			if err := pi.providerStore.SetExpirable(ctx, mh, true); err != nil {
				return nil, err
			}
		}
	}

	return results, nil
}

func (pi *ProviderIndexService) fetchFromIPNI(ctx context.Context, mh mh.Multihash, targetClaims []multicodec.Code) ([]model.ProviderResult, error) {
	var results []model.ProviderResult
	findRes, err := pi.findClient.Find(ctx, mh)
	if err != nil {
		return nil, err
	}

	for _, mhres := range findRes.MultihashResults {
		results = append(results, mhres.ProviderResults...)
	}

	results, err = filterCodecs(results, targetClaims)
	if err != nil {
		return nil, err
	}

	return results, nil
}

func (pi *ProviderIndexService) fetchFromLegacy(ctx context.Context, mh mh.Multihash, targetClaims []multicodec.Code) ([]model.ProviderResult, error) {
	results, err := pi.legacyClaims.Find(ctx, mh)
	if err != nil {
		return nil, err
	}

	results, err = filterCodecs(results, targetClaims)
	if err != nil {
		return nil, err
	}

	return results, nil
}

func filterCodecs(results []model.ProviderResult, codecs []multicodec.Code) ([]model.ProviderResult, error) {
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

func filterBySpace(results []model.ProviderResult, mh mh.Multihash, spaces []did.DID) ([]model.ProviderResult, error) {
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
		if !filterableByContextID(result) {
			return true, nil
		}
		return slices.ContainsFunc(encryptedIds, func(encyptedID types.EncodedContextID) bool {
			return bytes.Equal(result.ContextID, encyptedID)
		}), nil
	})
	if err != nil {
		return nil, err
	}
	return filtered, nil
}

func (pi *ProviderIndexService) Cache(ctx context.Context, provider peer.AddrInfo, contextID string, digests iter.Seq[mh.Multihash], meta meta.Metadata) error {
	// Cache the entries _with_ expiry - we cannot rely on the IPNI notifier to
	// tell us when they are published since we are not publishing to IPNI.
	return Cache(ctx, pi.providerStore, provider, contextID, digests, meta, true)
}

func Cache(ctx context.Context, providerStore types.ProviderStore, provider peer.AddrInfo, contextID string, digests iter.Seq[mh.Multihash], meta meta.Metadata, expire bool) error {
	log := log.With("contextID", []byte(contextID))
	log.Infof("caching provider results for provider: %s", provider.ID)

	mdb, err := meta.MarshalBinary()
	if err != nil {
		return fmt.Errorf("marshaling metadata: %w", err)
	}

	pr := model.ProviderResult{
		ContextID: []byte(contextID),
		Metadata:  mdb,
		Provider:  &provider,
	}

	var joberr error
	q := jobqueue.NewJobQueue(
		func(ctx context.Context, digest mh.Multihash) error {
			return addProviderResult(ctx, providerStore, digest, pr, expire)
		},
		jobqueue.WithConcurrency(5),
		jobqueue.WithErrorHandler(func(err error) { joberr = err }),
	)
	q.Startup()
	i := 0
	for d := range digests {
		err := q.Queue(ctx, d)
		if err != nil {
			return err
		}
		i++
	}
	err = q.Shutdown(ctx)
	if err != nil {
		return fmt.Errorf("shutting down job queue: %w", err)
	}
	if joberr != nil {
		return fmt.Errorf("appending provider result: %w", joberr)
	}

	log.Infof("cached %d provider results", i)
	return nil
}

// Publish should do the following:
// 1. Write the entries to the cache with no expiration until publishing is complete
// 2. Generate an advertisement for the advertised hashes and publish/announce it
func (pi *ProviderIndexService) Publish(ctx context.Context, provider peer.AddrInfo, contextID string, digests iter.Seq[mh.Multihash], meta meta.Metadata) error {
	log := log.With("contextID", []byte(contextID))

	// cache but do not expire (entries will be expired via the notifier)
	err := Cache(ctx, pi.providerStore, provider, contextID, digests, meta, false)
	if err != nil {
		return fmt.Errorf("caching provider results: %w", err)
	}

	id, err := pi.publisher.Publish(ctx, provider, contextID, digests, meta)
	if err != nil {
		return err
	}
	log.Infof("published IPNI advert: %s", id)
	return nil
}

func addProviderResult(ctx context.Context, providerStore types.ProviderStore, digest mh.Multihash, meta model.ProviderResult, expire bool) error {
	_, err := providerStore.Add(ctx, digest, meta)
	if err != nil {
		return fmt.Errorf("adding provider result for digest: %s: %w", digest.B58String(), err)
	}
	err = providerStore.SetExpirable(ctx, digest, expire)
	if err != nil {
		return fmt.Errorf("setting expirable for digest: %s: %w", digest.B58String(), err)
	}
	return nil
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

// filterableByContextID determines if the metadata can be filtered using a
// [types.ContextID].
func filterableByContextID(result model.ProviderResult) bool {
	md := metadata.MetadataContext.New()
	err := md.UnmarshalBinary(result.Metadata)
	if err != nil {
		log.Warnf("decoding metadata: %w", err)
		return false
	}
	// we're only able to filter results with location commitment metadata atm
	lcommMeta := md.Get(metadata.LocationCommitmentID)
	return lcommMeta != nil
}
