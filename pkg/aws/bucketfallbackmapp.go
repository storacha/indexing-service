package aws

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/ipfs/go-cid"
	multihash "github.com/multiformats/go-multihash"
	"github.com/storacha/go-capabilities/pkg/assert"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/go-ucanto/principal"
	"github.com/storacha/indexing-service/pkg/types"
)

type ContentToClaimsMapper interface {
	GetClaims(ctx context.Context, contentHash multihash.Multihash) ([]cid.Cid, error)
}

type BucketFallbackMapper struct {
	id               principal.Signer
	carparkPublicURL url.URL
	baseMapper       ContentToClaimsMapper
}

func NewBucketFallbackMapper(id principal.Signer, carparkPublicURL url.URL, baseMapper ContentToClaimsMapper) BucketFallbackMapper {
	return BucketFallbackMapper{
		id:               id,
		carparkPublicURL: carparkPublicURL,
		baseMapper:       baseMapper,
	}
}

func (cfm BucketFallbackMapper) GetClaims(ctx context.Context, contentHash multihash.Multihash) ([]cid.Cid, error) {
	claims, err := cfm.baseMapper.GetClaims(ctx, contentHash)
	if err == nil || !errors.Is(err, types.ErrKeyNotFound) {
		return claims, err
	}

	resp, err := http.DefaultClient.Head(cfm.carparkPublicURL.JoinPath(toBlobKey(contentHash)).String())
	if err != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, types.ErrKeyNotFound
	}
	size := uint64(resp.ContentLength)
	delegation, err := assert.Location.Delegate(
		cfm.id,
		cfm.id,
		cfm.id.DID().String(),
		assert.LocationCaveats{
			Content:  assert.FromHash(contentHash),
			Location: []url.URL{*cfm.carparkPublicURL.JoinPath(toBlobKey(contentHash))},
			Range:    &assert.Range{Offset: 0, Length: &size},
		},
		delegation.WithExpiration(int(time.Now().Add(time.Hour).Unix())),
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
		Codec:    cid.Raw,
		MhType:   multihash.IDENTITY,
		MhLength: len(delegationData),
	}.Sum(delegationData)
	if err != nil {
		return nil, fmt.Errorf("generating identity cid: %w", err)
	}
	return []cid.Cid{c}, err
}

func toBlobKey(contentHash multihash.Multihash) string {
	mhStr := contentHash.B58String()
	return fmt.Sprintf("%s/%s.blob", mhStr)
}
