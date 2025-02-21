package aws

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/multiformats/go-multibase"
	"github.com/multiformats/go-multicodec"
	multihash "github.com/multiformats/go-multihash"
	cassert "github.com/storacha/go-libstoracha/capabilities/assert"
	ctypes "github.com/storacha/go-libstoracha/capabilities/types"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/go-ucanto/principal"
	"github.com/storacha/indexing-service/pkg/service/providerindex"
)

type BlockIndexStore interface {
	Query(ctx context.Context, digest multihash.Multihash) ([]BlockIndexRecord, error)
}

type BlockIndexRecord struct {
	CarPath string
	Offset  uint64
	Length  uint64
}

type blockIndexTableMapper struct {
	id              principal.Signer
	blockIndexStore BlockIndexStore
	bucketURL       url.URL
}

var _ providerindex.ContentToClaimsMapper = blockIndexTableMapper{}

// NewBlockIndexTableMapper creates a new ContentToClaimsMapper that synthethizes location claims from data in the
// blockIndexStore - a giant index of historical data, mapping multihashes to bucket keys/URLs and their byte offsets.
//
// The data referenced by bucket keys in the blockIndexStore has been consolidated into a single bucket. So this
// instance does the work of mapping old bucket keys to URLs, where the base URL is the passed bucketURL param.
//
// Using the data in the blockIndexStore, the service will materialize content claims using the id param as the
// signing key.
func NewBlockIndexTableMapper(id principal.Signer, blockIndexStore BlockIndexStore, bucketURL string) (blockIndexTableMapper, error) {
	burl, err := url.Parse(bucketURL)
	if err != nil {
		return blockIndexTableMapper{}, fmt.Errorf("parsing bucket URL: %w", err)
	}

	return blockIndexTableMapper{
		id:              id,
		blockIndexStore: blockIndexStore,
		bucketURL:       *burl,
	}, nil
}

// GetClaims implements providerindex.ContentToClaimsMapper.
// Although it returns a list of CIDs, they are identity CIDs, so they contain the actual claims the refer to.
func (bit blockIndexTableMapper) GetClaims(ctx context.Context, contentHash multihash.Multihash) ([]cid.Cid, error) {
	var locs []cassert.LocationCaveats

	// lets see if we can materialize some location claims
	content := ctypes.FromHash(contentHash)
	records, err := bit.blockIndexStore.Query(ctx, content.Hash())
	if err != nil {
		return nil, err
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

			u = bit.bucketURL.JoinPath(fmt.Sprintf("/%s/%s.car", shard.String(), shard.String()))
			locs = append(locs, cassert.LocationCaveats{
				Content:  content,
				Location: []url.URL{*u},
				Range:    &cassert.Range{Offset: r.Offset, Length: &r.Length},
			})
		} else {
			locs = append(locs, cassert.LocationCaveats{
				Content:  content,
				Location: []url.URL{*u},
				Range:    &cassert.Range{Offset: r.Offset, Length: &r.Length},
			})
		}
	}

	claimCids := make([]cid.Cid, 0, len(locs))
	for _, loc := range locs {
		claim, err := cassert.Location.Delegate(
			bit.id,
			bit.id,
			bit.id.DID().String(),
			loc,
			delegation.WithExpiration(int(time.Now().Add(time.Hour).Unix())),
		)
		if err != nil {
			continue
		}

		claimData, err := io.ReadAll(claim.Archive())
		if err != nil {
			continue
		}

		c, err := cid.Prefix{
			Version:  1,
			Codec:    uint64(multicodec.Car),
			MhType:   multihash.IDENTITY,
			MhLength: len(claimData),
		}.Sum(claimData)
		if err != nil {
			continue
		}

		claimCids = append(claimCids, c)
	}

	return claimCids, nil
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
