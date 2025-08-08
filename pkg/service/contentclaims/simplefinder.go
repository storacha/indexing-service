package contentclaims

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/ipld/go-ipld-prime"
	"github.com/storacha/go-ucanto/core/delegation"
)

// simpleFinder is a read through cache for fetching content claims
type simpleFinder struct {
	httpClient *http.Client
}

var _ Finder = (*simpleFinder)(nil)

// ClaimFetchError is an error that occurred while attempting to fetch a claim
// from the network. This is typically a non-fatal error for indexer queries so
// this error type is public to allow detection via [errors.As].
type ClaimFetchError struct {
	err error
}

func (cfe ClaimFetchError) Error() string {
	return fmt.Errorf("fetching claim: %w", cfe.err).Error()
}

func (cfe ClaimFetchError) Unwrap() error {
	return cfe.err
}

// NewSimpleFinder creates a new [Finder] with the provided HTTP client.
func NewSimpleFinder(httpClient *http.Client) Finder {
	return &simpleFinder{
		httpClient: httpClient,
	}
}

// Find attempts to fetch a claim from the provided URL.
func (sf *simpleFinder) Find(ctx context.Context, id ipld.Link, fetchURL *url.URL) (delegation.Delegation, error) {
	// attempt to fetch the claim from provided url
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fetchURL.String(), nil)
	if err != nil {
		return nil, ClaimFetchError{fmt.Errorf("failed to create request: %w", err)}
	}

	resp, err := sf.httpClient.Do(req)
	if err != nil {
		return nil, ClaimFetchError{fmt.Errorf("failed to fetch claim: %w", err)}
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, ClaimFetchError{fmt.Errorf("reading fetched claim body: %w", err)}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, ClaimFetchError{fmt.Errorf("failure response fetching claim. URL: %s, status: %s, message: %s", fetchURL.String(), resp.Status, string(body))}
	}
	dlg, err := delegation.Extract(body)
	if err != nil {
		return nil, ClaimFetchError{fmt.Errorf("extracting delegation from archive: %w", err)}
	}
	if id.String() != dlg.Link().String() {
		return nil, ClaimFetchError{fmt.Errorf("received delegation: %s, does not match expected delegation: %s", dlg.Link(), id)}
	}
	return dlg, nil
}
