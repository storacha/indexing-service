package providerindex

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"iter"
	"slices"
	"sync"
	"time"

	"github.com/benbjohnson/clock"
	logging "github.com/ipfs/go-log/v2"
	ipnifind "github.com/ipni/go-libipni/find/client"
	"github.com/ipni/go-libipni/find/model"
	meta "github.com/ipni/go-libipni/metadata"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multicodec"
	mh "github.com/multiformats/go-multihash"
	"github.com/storacha/go-libstoracha/ipnipublisher/publisher"
	"github.com/storacha/go-libstoracha/metadata"
	"github.com/storacha/go-ucanto/did"
	"github.com/storacha/indexing-service/pkg/service/providerindex/legacy"
	"github.com/storacha/indexing-service/pkg/telemetry"
	"github.com/storacha/indexing-service/pkg/types"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	// MaxBatchSize is the maximum number of items that'll be added to a batch.
	MaxBatchSize = 10_000
	IPNITimeout  = 1500 * time.Millisecond
)

type QueryKey struct {
	Spaces       []did.DID
	Hash         mh.Multihash
	TargetClaims []multicodec.Code
}

// ProviderIndexService is a read/write interface to a local cache of providers that falls back to IPNI
type ProviderIndexService struct {
	providerStore   types.ProviderStore
	noProviderStore types.NoProviderStore
	findClient      ipnifind.Finder
	publisher       publisher.Publisher
	legacyClaims    legacy.ClaimsFinder
	mutex           sync.Mutex
	clock           clock.Clock
	log             logging.EventLogger
}

var _ ProviderIndex = (*ProviderIndexService)(nil)

type config struct {
	log   logging.EventLogger
	clock clock.Clock
}

// Option configures an ProviderIndex.
type Option func(conf *config)

// WithLogger configures the service to use the passed logger instead of the
// default logger.
func WithLogger(log logging.EventLogger) Option {
	return func(conf *config) {
		conf.log = log
	}
}

// WithClock configures the provider index with a mockable clock for testing.
func WithClock(clock clock.Clock) Option {
	return func(conf *config) {
		conf.clock = clock
	}
}

func New(providerStore types.ProviderStore, noProviderStore types.NoProviderStore, findClient ipnifind.Finder, publisher publisher.Publisher, legacyClaims legacy.ClaimsFinder, options ...Option) *ProviderIndexService {
	conf := config{}
	for _, option := range options {
		option(&conf)
	}
	if conf.clock == nil {
		conf.clock = clock.New()
	}
	if conf.log == nil {
		conf.log = logging.Logger("providerindex")
	}
	return &ProviderIndexService{
		providerStore:   providerStore,
		noProviderStore: noProviderStore,
		findClient:      findClient,
		publisher:       publisher,
		legacyClaims:    legacyClaims,
		clock:           conf.clock,
		log:             conf.log,
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
	ctx, s := telemetry.StartSpan(ctx, "ProviderIndexService.Find")
	defer s.End()

	s.AddEvent("finding ProviderResults")
	results, err := pi.getProviderResults(ctx, qk.Hash, qk.TargetClaims)
	if err != nil {
		telemetry.Error(s, err, "finding ProviderResults")
		return nil, err
	}

	s.AddEvent("filtering results by space")
	return filterBySpace(results, qk.Hash, qk.Spaces)
}

func (pi *ProviderIndexService) getProviderResults(ctx context.Context, mh mh.Multihash, targetClaims []multicodec.Code) ([]model.ProviderResult, error) {
	ctx, s := telemetry.StartSpan(ctx, "ProviderIndexService.getProviderResults")
	defer s.End()

	s.AddEvent("searching in cache")
	res, err := pi.providerStore.Members(ctx, mh)
	if err == nil {
		res, _ = filterCodecs(res, targetClaims)
		if len(res) > 0 {
			s.AddEvent("cache hit")
			return res, nil
		}
	} else {
		if !errors.Is(err, types.ErrKeyNotFound) {
			telemetry.Error(s, err, "fetching from cache")
			return nil, err
		}
	}

	type queryResult struct {
		results []model.ProviderResult
		err     error
	}

	// buffered channels so goroutines don't block.
	ipniCh := make(chan queryResult, 1)
	legacyCh := make(chan queryResult, 1)

	// Create a cancelable context for the legacy query.
	legacyCtx, cancelLegacy := context.WithCancel(ctx)
	defer cancelLegacy()

	// Start IPNI query.
	go func() {
		s.AddEvent("fetching from IPNI")
		r, err := pi.fetchFromIPNI(ctx, s, mh, targetClaims)
		s.AddEvent("fetched from IPNI", trace.WithAttributes(attribute.Bool("found", len(r) != 0)))
		ipniCh <- queryResult{results: r, err: err}
	}()

	// Start legacy query.
	go func() {
		s.AddEvent("fetching from legacy services")
		r, err := pi.legacyClaims.Find(legacyCtx, mh, targetClaims)
		s.AddEvent("fetched from legacy services", trace.WithAttributes(attribute.Bool("found", len(r) != 0)))
		legacyCh <- queryResult{results: r, err: err}
	}()

	var ipniRes, legacyRes queryResult

	// Wait for both responses.
	for i := 0; i < 2; i++ {
		select {
		case res := <-ipniCh:
			ipniRes = res
			// If IPNI returns valid data, cancel the legacy lookup.
			if res.err == nil && len(res.results) > 0 {
				cancelLegacy()
			}
		case res := <-legacyCh:
			legacyRes = res
		}
	}

	// Prioritize IPNI results.
	if ipniRes.err == nil && len(ipniRes.results) > 0 {
		pi.cacheResults(ctx, s, mh, ipniRes.results)
		return ipniRes.results, nil
	}
	if legacyRes.err == nil && len(legacyRes.results) > 0 {
		pi.cacheResults(ctx, s, mh, legacyRes.results)
		return legacyRes.results, nil
	}

	// Neither query returned data: if error(s) is/are present join them and return as one wrapped error.
	// NB(forrest): it is also acceptable to return no result and no error in the event nothing was found.
	var queryError error
	if ipniRes.err != nil {
		queryError = errors.Join(queryError, fmt.Errorf("fetching from IPNI failed: %w", ipniRes.err))
	}
	if legacyRes.err != nil {
		queryError = errors.Join(queryError, fmt.Errorf("fetching from legacy services failed: %w", legacyRes.err))
	}
	return nil, queryError
}

// Helper function to cache results.
func (pi *ProviderIndexService) cacheResults(ctx context.Context, s trace.Span, mh mh.Multihash, results []model.ProviderResult) {
	s.AddEvent("caching results")
	n, err := pi.providerStore.Add(ctx, mh, results...)
	if err != nil {
		telemetry.Error(s, err, "caching results")
		pi.log.Errorf("adding results to set: %s", err)
		return
	}
	if n > 0 {
		if err := pi.providerStore.SetExpirable(ctx, mh, true); err != nil {
			telemetry.Error(s, err, "setting cache entry expiration")
			pi.log.Errorf("setting expirable: %s", err)
		}
	}
}

// Helper function to cache empty results.
func (pi *ProviderIndexService) cacheNoProviderResults(ctx context.Context, s trace.Span, mh mh.Multihash, targetClaims []multicodec.Code) {
	s.AddEvent("caching no results")
	n, err := pi.noProviderStore.Add(ctx, mh, targetClaims...)
	if err != nil {
		telemetry.Error(s, err, "caching no provider results")
		pi.log.Errorf("caching no results: %s", err)
		return
	}

	if n > 0 {
		if err := pi.noProviderStore.SetExpirable(ctx, mh, true); err != nil {
			telemetry.Error(s, err, "setting no provider results expiration")
			pi.log.Errorf("setting no results expirable: %s", err)
		}
	}
}

func (pi *ProviderIndexService) fetchFromIPNI(ctx context.Context, s trace.Span, mh mh.Multihash, targetClaims []multicodec.Code) ([]model.ProviderResult, error) {
	var results []model.ProviderResult

	// check if we already know there are no results in IPNI
	codes, err := pi.noProviderStore.Members(ctx, mh)
	if err == nil {
		missingClaims, _ := filter(targetClaims, func(targetCode multicodec.Code) (bool, error) {
			return !slices.Contains(codes, targetCode), nil
		})
		if len(missingClaims) == 0 {
			return nil, nil
		}
	} else {
		if !errors.Is(err, types.ErrKeyNotFound) {
			telemetry.Error(s, err, "fetching from cache")
			return nil, err
		}
	}

	// IPNI will occassionally hang. If it does, don't wait for it.
	ctx, cancel := pi.clock.WithTimeout(ctx, IPNITimeout)
	defer cancel()

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
	if len(results) == 0 {
		pi.cacheNoProviderResults(ctx, s, mh, targetClaims)
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
	return Cache(ctx, pi.log, pi.providerStore, provider, contextID, digests, meta, true)
}

func Cache(ctx context.Context, log logging.EventLogger, providerStore types.ProviderStore, provider peer.AddrInfo, contextID string, digests iter.Seq[mh.Multihash], meta meta.Metadata, expire bool) error {
	log.Infof("caching provider results for context: %s, provider: %s", contextID, provider.ID)
	ctx, s := telemetry.StartSpan(ctx, "ProviderIndexService.Cache")
	defer s.End()

	mdb, err := meta.MarshalBinary()
	if err != nil {
		return fmt.Errorf("marshaling metadata: %w", err)
	}

	pr := model.ProviderResult{
		ContextID: []byte(contextID),
		Metadata:  mdb,
		Provider:  &provider,
	}

	batch := providerStore.Batch()
	size := 0
	total := 0
	for d := range digests {
		err := batch.Add(ctx, d, pr)
		if err != nil {
			return err
		}
		err = batch.SetExpirable(ctx, d, expire)
		if err != nil {
			return err
		}
		total++

		size++
		if size >= MaxBatchSize {
			s.AddEvent("commit batch")
			err := batch.Commit(ctx)
			if err != nil {
				return fmt.Errorf("comitting batch: %w", err)
			}
			batch = providerStore.Batch()
			size = 0
		}
	}

	if size > 0 {
		s.AddEvent("commit batch")
		err = batch.Commit(ctx)
		if err != nil {
			return fmt.Errorf("comitting batch: %w", err)
		}
	}

	s.SetAttributes(attribute.KeyValue{Key: "total", Value: attribute.IntValue(total)})
	log.Infof("cached %d provider results for context: %s", total, contextID)
	return nil
}

// Publish should do the following:
// 1. Write the entries to the cache with no expiration until publishing is complete
// 2. Generate an advertisement for the advertised hashes and publish/announce it
func (pi *ProviderIndexService) Publish(ctx context.Context, provider peer.AddrInfo, contextID string, digests iter.Seq[mh.Multihash], meta meta.Metadata) error {
	ctx, s := telemetry.StartSpan(ctx, "ProviderIndexService.Publish")
	defer s.End()

	// cache but do not expire (entries will be expired via the notifier)
	s.AddEvent("start pre-cache")
	err := Cache(ctx, pi.log, pi.providerStore, provider, contextID, digests, meta, false)
	if err != nil {
		return fmt.Errorf("caching provider results: %w", err)
	}

	pi.mutex.Lock()
	defer pi.mutex.Unlock()

	s.AddEvent("start publish")
	id, err := pi.publisher.Publish(ctx, provider, contextID, digests, meta)
	if err != nil {
		if errors.Is(err, publisher.ErrAlreadyAdvertised) {
			// skipping is ok in this case
			pi.log.Warnf("Skipping previously published advert")
			return nil
		}

		return fmt.Errorf("publishing advert: %w", err)
	}
	pi.log.Infof("published IPNI advert: %s", id)
	return nil
}

func filter[T any](results []T, filterFunc func(T) (bool, error)) ([]T, error) {

	filtered := make([]T, 0, len(results))
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
		return false
	}
	// we're only able to filter results with location commitment metadata atm
	lcommMeta := md.Get(metadata.LocationCommitmentID)
	return lcommMeta != nil
}
