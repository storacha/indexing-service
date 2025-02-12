package legacy

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime/datamodel"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multibase"
	"github.com/multiformats/go-multicodec"
	"github.com/multiformats/go-multihash"
	cassert "github.com/storacha/go-capabilities/pkg/assert"
	ctypes "github.com/storacha/go-capabilities/pkg/types"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/go-ucanto/core/ipld"
	"github.com/storacha/go-ucanto/principal"
	"github.com/storacha/indexing-service/pkg/blobindex"
	"github.com/storacha/indexing-service/pkg/bytemap"
	"github.com/storacha/indexing-service/pkg/internal/link"
	"github.com/storacha/indexing-service/pkg/service/queryresult"
	"github.com/storacha/indexing-service/pkg/types"
)

type IndexingService struct {
	id              principal.Signer
	indexingService types.Service
	blockIndexStore BlockIndexStore
	bucketURL       url.URL
}

func (l *IndexingService) Cache(ctx context.Context, provider peer.AddrInfo, claim delegation.Delegation) error {
	return l.indexingService.Cache(ctx, provider, claim)
}

func (l *IndexingService) Get(ctx context.Context, claim datamodel.Link) (delegation.Delegation, error) {
	return l.indexingService.Get(ctx, claim)
}

func (l *IndexingService) Publish(ctx context.Context, claim delegation.Delegation) error {
	return l.indexingService.Publish(ctx, claim)
}

func (l *IndexingService) Query(ctx context.Context, q types.Query) (types.QueryResult, error) {
	// Create a cancellable context for querying the indexStore so we can stop it early if the indexService succeeds.
	bisCtx, cancelBis := context.WithCancel(ctx)
	defer cancelBis()

	// We'll capture the results of indexStore query in a channel, using the result if the indexService
	// doesn't yield anything.
	type indexResult struct {
		claims  map[cid.Cid]delegation.Delegation
		indexes bytemap.ByteMap[types.EncodedContextID, blobindex.ShardedDagIndexView]
		err     error
	}
	indexResCh := make(chan indexResult, 1)

	// Query the indexStore async.
	go func() {
		var locs []cassert.LocationCaveats
		for _, h := range q.Hashes {
			// lets see if we can materialize some location claims
			content := ctypes.FromHash(h)
			records, err := l.blockIndexStore.Query(bisCtx, content.Hash())
			if err != nil {
				if errors.Is(err, types.ErrKeyNotFound) {
					continue
				}
				// bail if non-continuable error.
				indexResCh <- indexResult{err: err}
				return
			}

			for _, r := range records {
				u, err := url.Parse(r.CarPath)
				if err != nil || !u.IsAbs() {
					// non-URL is legacy region/bucket/key format
					// e.g. us-west-2/dotstorage-prod-1/raw/bafy...
					parts := strings.Split(r.CarPath, "/")
					key := strings.Join(parts[2:], "/")
					shard, err := bucketKeyToShardLink(key)
					if err != nil {
						continue
					}

					u = l.bucketURL.JoinPath(fmt.Sprintf("/%s/%s.car", shard.String(), shard.String()))
					locs = append(locs, cassert.LocationCaveats{
						Content:  content,
						Location: []url.URL{*u},
						Range:    &cassert.Range{Offset: r.Offset, Length: &r.Length},
					})
					continue
				}

				locs = append(locs, cassert.LocationCaveats{
					Content:  content,
					Location: []url.URL{*u},
					Range:    &cassert.Range{Offset: r.Offset, Length: &r.Length},
				})
			}
		}

		claims := make(map[cid.Cid]delegation.Delegation, len(locs))
		for _, loc := range locs {
			claim, err := cassert.Location.Delegate(
				l.id,
				l.id,
				l.id.DID().String(),
				loc,
				delegation.WithExpiration(int(time.Now().Add(time.Hour).Unix())),
			)
			if err != nil {
				indexResCh <- indexResult{err: err}
				return
			}
			claims[link.ToCID(claim.Link())] = claim
		}

		indexes := bytemap.NewByteMap[types.EncodedContextID, blobindex.ShardedDagIndexView](0)
		indexResCh <- indexResult{claims: claims, indexes: indexes, err: nil}
	}()

	results, err := l.indexingService.Query(ctx, q)
	if err != nil {
		// if we fail to query the indexService, kill the indexStore query and bail with error.
		// cancellation of the indexStore query is handled in defer statement at top of method.
		return nil, err
	}
	if len(results.Claims()) > 0 || len(results.Indexes()) > 0 || len(q.Hashes) == 0 {
		// indexService produced a result, kill the blockIndex query and return results.
		// cancellation of the blockStore query is handled in defer statement at top of method.
		return results, nil
	}

	// indexService query yields empty result, check the indexStore.

	bsRes := <-indexResCh
	if bsRes.err != nil {
		return nil, bsRes.err
	}

	// the indexService yielded an empty result, use the indexStore query result.
	return queryresult.Build(bsRes.claims, bsRes.indexes)
}

var _ types.Service = (*IndexingService)(nil)

type BlockIndexRecord struct {
	CarPath string
	Offset  uint64
	Length  uint64
}

type BlockIndexStore interface {
	Query(ctx context.Context, digest multihash.Multihash) ([]BlockIndexRecord, error)
}

// NewService creates a new indexing service that wraps the passed service and
// transparently proxies to it, with the exception of the call to [Query], which
// calls the wrapped service and then inspects the results. If they are empty
// then it will query the blockIndexStore - a giant index of historical data,
// mapping multihashes to bucket keys/URLs and their byte offsets.
//
// The data referenced by bucket keys in the blockIndexStore has been
// consolidated into a single bucket. So this instance does the work of mapping
// old bucket keys to URLs, where the base URL is the passed bucketURL param.
//
// Using the data in the blockIndexStore, the service will materialize content
// claims using the id param as the signing key, and add them to the query
// results before returning them back to the caller.
func NewService(id principal.Signer, indexer types.Service, blockIndexStore BlockIndexStore, bucketURL string) (*IndexingService, error) {
	burl, err := url.Parse(bucketURL)
	if err != nil {
		return nil, fmt.Errorf("parsing bucket URL: %w", err)
	}
	return &IndexingService{id, indexer, blockIndexStore, *burl}, nil
}

func bucketKeyToShardLink(key string) (ipld.Link, error) {
	parts := strings.Split(key, "/")
	filename := parts[len(parts)-1]
	hash := strings.Split(filename, ".")[0]

	// recent buckets encode CAR CID in filename
	shard, err := cid.Parse(hash)
	if err != nil {
		// older buckets base32 encode a CAR multihash <base32(car-multihash)>.car
		_, digestBytes, err := multibase.Decode(string(multibase.Base32) + hash)
		if err != nil {
			return nil, err
		}
		digest, err := multihash.Cast(digestBytes)
		if err != nil {
			return nil, err
		}
		return cidlink.Link{Cid: cid.NewCidV1(uint64(multicodec.Car), digest)}, nil
	}
	if shard.Prefix().Codec != uint64(multicodec.Car) {
		return nil, errors.New("not a CAR CID")
	}
	return cidlink.Link{Cid: shard}, nil
}
