package contentclaims

import (
	"context"
	"net/url"

	"github.com/ipld/go-ipld-prime"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/multiformats/go-multihash"
	"github.com/storacha/go-ucanto/core/delegation"
)

type identityCidFinder struct {
	finder Finder
}

var _ Finder = (*cachingFinder)(nil)

// WithIdentityCids augments a ClaimFinder with claims retrieved automatically whenever an identity CID is used
func WithIdentityCids(finder Finder) Finder {
	return &identityCidFinder{finder}
}

// Find attempts to fetch a claim from either the permenant storage or via the provided URL
func (idf *identityCidFinder) Find(ctx context.Context, id ipld.Link, fetchURL url.URL) (delegation.Delegation, error) {

	if cidLink, ok := id.(cidlink.Link); ok {
		if cidLink.Cid.Prefix().MhType == multihash.IDENTITY {
			dh, err := multihash.Decode(cidLink.Cid.Hash())
			if err != nil {
				return nil, err
			}
			return delegation.Extract(dh.Digest)
		}
	}

	// attempt to fetch the claim from the underlying claim finder
	return idf.finder.Find(ctx, id, fetchURL)
}
