package aws

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multicodec"
	multihash "github.com/multiformats/go-multihash"
	cassert "github.com/storacha/go-capabilities/pkg/assert"
	ctypes "github.com/storacha/go-capabilities/pkg/types"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/go-ucanto/principal"
	"github.com/storacha/indexing-service/pkg/internal/digestutil"
	"github.com/storacha/indexing-service/pkg/types"
)

type ContentToClaimsMapper interface {
	GetClaims(ctx context.Context, contentHash multihash.Multihash) ([]cid.Cid, error)
}

type BucketFallbackMapper struct {
	id         principal.Signer
	bucketURL  *url.URL
	baseMapper ContentToClaimsMapper
	getOpts    func() []delegation.Option
}

func NewBucketFallbackMapper(id principal.Signer, bucketURL *url.URL, baseMapper ContentToClaimsMapper, getOpts func() []delegation.Option) BucketFallbackMapper {
	return BucketFallbackMapper{
		id:         id,
		bucketURL:  bucketURL,
		baseMapper: baseMapper,
		getOpts:    getOpts,
	}
}

func (cfm BucketFallbackMapper) GetClaims(ctx context.Context, contentHash multihash.Multihash) ([]cid.Cid, error) {
	claims, err := cfm.baseMapper.GetClaims(ctx, contentHash)
	if err == nil || !errors.Is(err, types.ErrKeyNotFound) {
		return claims, err
	}

	resp, err := http.DefaultClient.Head(cfm.bucketURL.JoinPath(toBlobKey(contentHash)).String())
	if err != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, types.ErrKeyNotFound
	}
	size := uint64(resp.ContentLength)
	delegation, err := cassert.Location.Delegate(
		cfm.id,
		cfm.id,
		cfm.id.DID().String(),
		cassert.LocationCaveats{
			Content:  ctypes.FromHash(contentHash),
			Location: []url.URL{*cfm.bucketURL.JoinPath(toBlobKey(contentHash))},
			Range:    &cassert.Range{Offset: 0, Length: &size},
		},
		cfm.getOpts()...,
	)
	if err != nil {
		return nil, fmt.Errorf("generating delegation: %w", err)
	}
	delegationData, err := io.ReadAll(delegation.Archive())
	if err != nil {
		return nil, fmt.Errorf("serializing delegation: %w", err)
	}
	c, err := cid.Prefix{
		Version:  1,
		Codec:    uint64(multicodec.Car),
		MhType:   multihash.IDENTITY,
		MhLength: len(delegationData),
	}.Sum(delegationData)
	if err != nil {
		return nil, fmt.Errorf("generating identity cid: %w", err)
	}
	return []cid.Cid{c}, err
}

func toBlobKey(contentHash multihash.Multihash) string {
	mhStr := digestutil.Format(contentHash)
	return fmt.Sprintf("%s/%s.blob", mhStr, mhStr)
}
