package providerindex

import (
	"bytes"
	"context"
	"fmt"
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
}

var _ ProviderIndex = (*ProviderIndexService)(nil)

// TBD access to legacy systems
type LegacySystems interface{}

func New(providerStore types.ProviderStore, findClient ipnifind.Finder, publisher publisher.Publisher, legacySystems LegacySystems) *ProviderIndexService {
	return &ProviderIndexService{
		providerStore: providerStore,
		findClient:    findClient,
		publisher:     publisher,
	}
}

// Find should do the following
//  1. Read from the IPNI Storage cache to get a list of providers
//     a. If there is no record in cache, query IPNI, filter out any non-content claims metadata, and store
//     the resulting records in the cache
//     b. the are no records in the cache or IPNI, it can attempt to read from legacy systems -- Dynamo tables & content claims storage, synthetically constructing provider results
//  2. With returned provider results, filter additionally for claim type. If space dids are set, calculate an encodedcontextid's by hashing space DID and Hash, and filter for a matching context id
//     Future TODO: kick off a conversion task to update the recrds
func (pi *ProviderIndexService) Find(ctx context.Context, qk QueryKey) ([]model.ProviderResult, error) {
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

func (pi *ProviderIndexService) getProviderResults(ctx context.Context, mh mh.Multihash) ([]model.ProviderResult, error) {
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

func (pi *ProviderIndexService) filteredCodecs(results []model.ProviderResult, codecs []multicodec.Code) ([]model.ProviderResult, error) {
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

func (pi *ProviderIndexService) filterBySpace(results []model.ProviderResult, mh mh.Multihash, spaces []did.DID) ([]model.ProviderResult, error) {
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

func (pi *ProviderIndexService) Cache(ctx context.Context, provider peer.AddrInfo, contextID string, digests []mh.Multihash, meta meta.Metadata) error {
	// Cache the entries _with_ expiry - we cannot rely on the IPNI notifier to
	// tell us when they are published since we are not publishing to IPNI.
	return Cache(ctx, pi.providerStore, provider, contextID, digests, meta, true)
}

func Cache(ctx context.Context, providerStore types.ProviderStore, provider peer.AddrInfo, contextID string, digests []mh.Multihash, meta meta.Metadata, expire bool) error {
	log := log.With("contextID", []byte(contextID))
	log.Infof("caching %d provider results for provider: %s", len(digests), provider.ID)

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
			return appendProviderResult(ctx, providerStore, digest, pr, expire)
		},
		jobqueue.WithConcurrency(5),
		jobqueue.WithErrorHandler(func(err error) { joberr = err }),
	)
	q.Startup()
	for _, d := range digests {
		err := q.Queue(ctx, d)
		if err != nil {
			return err
		}
	}
	err = q.Shutdown(ctx)
	if err != nil {
		return fmt.Errorf("shutting down job queue: %w", err)
	}
	if joberr != nil {
		return fmt.Errorf("appending provider result: %w", joberr)
	}

	log.Infof("cached %d provider results", len(digests))
	return nil
}

// Publish should do the following:
// 1. Write the entries to the cache with no expiration until publishing is complete
// 2. Generate an advertisement for the advertised hashes and publish/announce it
func (pi *ProviderIndexService) Publish(ctx context.Context, provider peer.AddrInfo, contextID string, digests []mh.Multihash, meta meta.Metadata) error {
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

// TODO: atomic append...
func appendProviderResult(ctx context.Context, providerStore types.ProviderStore, digest mh.Multihash, meta model.ProviderResult, expire bool) error {
	metas, err := providerStore.Get(ctx, digest)
	if err != nil {
		if err != types.ErrKeyNotFound {
			return fmt.Errorf("getting existing provider results for digest: %s: %w", digest.B58String(), err)
		}
	}
	metas = append(metas, meta)
	err = providerStore.Set(ctx, digest, metas, expire)
	if err != nil {
		return fmt.Errorf("setting provider results for digest: %s: %w", digest.B58String(), err)
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
