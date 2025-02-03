package aws

import (
	"context"
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

type BucketFallbackMapper struct {
	id         principal.Signer
	httpClient *http.Client
	bucketURL  *url.URL
	getOpts    func() []delegation.Option
}

func NewBucketFallbackMapper(id principal.Signer, httpClient *http.Client, bucketURL *url.URL, getOpts func() []delegation.Option) BucketFallbackMapper {
	return BucketFallbackMapper{
		id:         id,
		httpClient: httpClient,
		bucketURL:  bucketURL,
		getOpts:    getOpts,
	}
}

func (cfm BucketFallbackMapper) GetClaims(ctx context.Context, contentHash multihash.Multihash) ([]cid.Cid, error) {
	resp, err := cfm.httpClient.Head(cfm.bucketURL.JoinPath(toBlobKey(contentHash)).String())
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
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
	return []cid.Cid{c}, nil
}

func toBlobKey(contentHash multihash.Multihash) string {
	mhStr := digestutil.Format(contentHash)
	return fmt.Sprintf("%s/%s.blob", mhStr, mhStr)
}
