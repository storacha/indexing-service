package contentclaims

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/ipld/go-ipld-prime"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/indexing-service/pkg/internal/link"
	"github.com/storacha/indexing-service/pkg/types"
)

type ClaimService struct {
	store  types.ContentClaimsStore
	cache  types.ContentClaimsCache
	finder Finder
}

var _ Service = (*ClaimService)(nil)

func (cs *ClaimService) Cache(ctx context.Context, claim delegation.Delegation) error {
	return cs.cache.Set(ctx, link.ToCID(claim.Link()), claim, true)
}

func (cs *ClaimService) Find(ctx context.Context, claim ipld.Link, url url.URL) (delegation.Delegation, error) {
	return cs.finder.Find(ctx, claim, url)
}

func (cs *ClaimService) Get(ctx context.Context, claim ipld.Link) (delegation.Delegation, error) {
	c, err := cs.cache.Get(ctx, link.ToCID(claim))
	if err == nil {
		return c, nil
	}
	if err != types.ErrKeyNotFound {
		return nil, err
	}
	c, err = cs.store.Get(ctx, claim)
	if err != nil {
		return nil, fmt.Errorf("getting claim from store: %w", err)
	}
	err = cs.Cache(ctx, c)
	if err != nil {
		return nil, fmt.Errorf("caching claim: %w", err)
	}
	return c, nil
}

func (cs *ClaimService) Publish(ctx context.Context, claim delegation.Delegation) error {
	err := cs.store.Put(ctx, claim.Link(), claim)
	if err != nil {
		return fmt.Errorf("putting claim to store: %w", err)
	}
	return cs.Cache(ctx, claim)
}

func New(store types.ContentClaimsStore, cache types.ContentClaimsCache, httpClient *http.Client) *ClaimService {
	f := WithCache(WithStore(NewSimpleFinder(httpClient), store), cache)
	return &ClaimService{store, cache, f}
}
