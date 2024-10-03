package claimlookup_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/ipfs/go-cid"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/storacha-network/go-ucanto/core/delegation"
	"github.com/storacha-network/indexing-service/pkg/internal/testutil"
	"github.com/storacha-network/indexing-service/pkg/service/claimlookup"
	"github.com/storacha-network/indexing-service/pkg/types"
	"github.com/stretchr/testify/require"
)

// MockContentClaimsStore is a mock implementation of the ContentClaimsStore interface
type MockContentClaimsStore struct {
	setErr, getErr error
	claims         map[string]delegation.Delegation
}

var _ types.ContentClaimsStore = &MockContentClaimsStore{}

// SetExpirable implements types.ContentClaimsStore.
func (m *MockContentClaimsStore) SetExpirable(ctx context.Context, key cid.Cid, expires bool) error {
	return nil
}

func (m *MockContentClaimsStore) Get(ctx context.Context, claimCid cid.Cid) (delegation.Delegation, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	claim, exists := m.claims[claimCid.String()]
	if !exists {
		return nil, types.ErrKeyNotFound
	}
	return claim, nil
}

func (m *MockContentClaimsStore) Set(ctx context.Context, claimCid cid.Cid, claim delegation.Delegation, overwrite bool) error {
	if m.setErr != nil {
		return m.setErr
	}
	m.claims[claimCid.String()] = claim
	return nil
}

func TestClaimLookup_LookupClaim(t *testing.T) {
	// Initialize mock store and HTTP client

	// Create a test CID
	cachedCid := testutil.RandomCID().(cidlink.Link).Cid
	notCachedCid := testutil.RandomCID().(cidlink.Link).Cid
	// Create a cached claim
	cachedClaim := testutil.RandomLocationDelection()
	notCachedClaim := testutil.RandomIndexDelegation()

	// sample error
	anError := errors.New("something went wrong")
	// Define test cases
	testCases := []struct {
		name          string
		claimCid      cid.Cid
		setErr        error
		getErr        error
		httpHandler   http.HandlerFunc
		expectedErr   error
		expectedClaim delegation.Delegation
		finalState    map[string]delegation.Delegation
	}{
		{
			name:          "Claim cached",
			claimCid:      cachedCid,
			expectedClaim: cachedClaim,
			finalState: map[string]delegation.Delegation{
				cachedCid.String(): cachedClaim,
			},
		},
		{
			name:          "Claim not cached, successful fetch",
			claimCid:      notCachedCid,
			expectedClaim: notCachedClaim,
			finalState: map[string]delegation.Delegation{
				cachedCid.String():    cachedClaim,
				notCachedCid.String(): notCachedClaim,
			},
		},
		{
			name:          "Lookup error",
			claimCid:      cachedCid,
			expectedClaim: nil,
			getErr:        anError,
			expectedErr:   fmt.Errorf("reading from claim cache: %w", anError),
			finalState: map[string]delegation.Delegation{
				cachedCid.String(): cachedClaim,
			},
		},
		{
			name:          "Save cache error",
			claimCid:      notCachedCid,
			expectedClaim: nil,
			setErr:        anError,
			expectedErr:   fmt.Errorf("caching fetched claim: %w", anError),
			finalState: map[string]delegation.Delegation{
				cachedCid.String(): cachedClaim,
			},
		},
		{
			name:          "http response error",
			claimCid:      notCachedCid,
			expectedClaim: nil,
			httpHandler:   http.NotFound,
			expectedErr:   errors.New("failure response fetching claim. status: 404 Not Found, message: 404 page not found\n"),
			finalState: map[string]delegation.Delegation{
				cachedCid.String(): cachedClaim,
			},
		},
	}

	// Run test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockStore := &MockContentClaimsStore{
				setErr: tc.setErr,
				getErr: tc.getErr,
				claims: map[string]delegation.Delegation{
					cachedCid.String(): cachedClaim,
				},
			}
			// generate a test server for requests
			handler := tc.httpHandler
			if handler == nil {
				handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					claimBytes := testutil.Must(io.ReadAll(notCachedClaim.Archive()))(t)
					testutil.Must(w.Write(claimBytes))(t)
				})
			}
			testServer := httptest.NewServer(handler)
			defer func() { testServer.Close() }()

			// Create ClaimLookup instance
			cl := claimlookup.NewClaimLookup(mockStore, testServer.Client())

			claim, err := cl.LookupClaim(context.Background(), tc.claimCid, *testutil.Must(url.Parse(testServer.URL))(t))
			if tc.expectedErr != nil {
				require.EqualError(t, err, tc.expectedErr.Error())
			} else {
				require.NoError(t, err)
			}
			testutil.RequireEqualDelegation(t, tc.expectedClaim, claim)
			finalState := tc.finalState
			if finalState == nil {
				finalState = make(map[string]delegation.Delegation)
			}
			require.Equal(t, len(finalState), len(mockStore.claims))
			for c, claim := range mockStore.claims {
				expectedClaim := finalState[c]
				testutil.RequireEqualDelegation(t, expectedClaim, claim)
			}
		})
	}
}
