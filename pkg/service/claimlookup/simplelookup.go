package claimlookup

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/ipfs/go-cid"
	"github.com/storacha/go-ucanto/core/delegation"
)

// simpleLookup is a read through cache for fetching content claims
type simpleLookup struct {
	httpClient *http.Client
}

var _ ClaimLookup = (*simpleLookup)(nil)

// NewSimpleClaimLookup creates a new ClaimLookup with the provided claimstore and HTTP client
func NewSimpleClaimLookup(httpClient *http.Client) ClaimLookup {
	return &simpleLookup{
		httpClient: httpClient,
	}
}

// LookupClaim attempts to fetch a claim from either the local cache or via the provided URL (caching the result if its fetched)
func (sl *simpleLookup) LookupClaim(ctx context.Context, claimCid cid.Cid, fetchURL url.URL) (delegation.Delegation, error) {
	// attempt to fetch the claim from provided url
	resp, err := sl.httpClient.Get(fetchURL.String())
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
	return delegation.Extract(body)
}
