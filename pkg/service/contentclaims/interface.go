package contentclaims

import (
	"context"
	"net/url"

	"github.com/ipld/go-ipld-prime"
	"github.com/storacha/go-ucanto/core/delegation"
)

type Finder interface {
	// Find and retrieve a claim via URL.
	Find(ctx context.Context, claim ipld.Link, fetchURL *url.URL) (delegation.Delegation, error)
}

type Service interface {
	// Get reads the claim from the cache, or from storage if not found.
	Get(ctx context.Context, claim ipld.Link) (delegation.Delegation, error)
	// Find attempts to read the claim from the cache, falling back to retrieving
	// it from storage and finally, if still not found, fetching it from the
	// provided URL. The result is stored in the cache.
	Find(ctx context.Context, claim ipld.Link, fetchURL *url.URL) (delegation.Delegation, error)
	// Cache writes the claim to the cache with default expiry.
	Cache(ctx context.Context, claim delegation.Delegation) error
	// Publish writes the claim to the cache, and adds it to storage.
	Publish(ctx context.Context, claim delegation.Delegation) error
}
