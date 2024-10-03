package claimlookup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/ipfs/go-cid"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/indexing-service/pkg/types"
)

// ClaimLookup is a read through cache for fetching content claims
type ClaimLookup struct {
	httpClient *http.Client
	claimStore types.ContentClaimsStore
}

// NewClaimLookup creates a new ClaimLookup with the provided claimstore and HTTP client
func NewClaimLookup(claimStore types.ContentClaimsStore, httpClient *http.Client) *ClaimLookup {
	return &ClaimLookup{
		httpClient: httpClient,
		claimStore: claimStore,
	}
}

// LookupClaim attempts to fetch a claim from either the local cache or via the provided URL (caching the result if its fetched)
func (cl *ClaimLookup) LookupClaim(ctx context.Context, claimCid cid.Cid, fetchURL url.URL) (delegation.Delegation, error) {
	// attempt to read claim from cache and return it if succesful
	claim, err := cl.claimStore.Get(ctx, claimCid)
	if err == nil {
		return claim, nil
	}

	// if an error occurred other than the claim not being in the cache, return it
	if !errors.Is(err, types.ErrKeyNotFound) {
		return nil, fmt.Errorf("reading from claim cache: %w", err)
	}

	// attempt to fetch the claim from provided url
	resp, err := cl.httpClient.Get(fetchURL.String())
	if err != nil {
		return nil, fmt.Errorf("failed to fetch claim: %w", err)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading fetched claim body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("failure response fetching claim. status: %s, message: %s", resp.Status, string(body))
	}
	fmt.Println(len(body))
	claim, err = delegation.Extract(body)
	if err != nil {
		return nil, fmt.Errorf("extracting claim from response: %w", err)
	}

	// cache the claim for the future
	if err := cl.claimStore.Set(ctx, claimCid, claim, true); err != nil {
		return nil, fmt.Errorf("caching fetched claim: %w", err)
	}
	return claim, nil
}
