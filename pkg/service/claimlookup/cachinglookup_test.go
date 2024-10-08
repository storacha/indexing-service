package claimlookup_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"testing"

	"github.com/ipfs/go-cid"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/indexing-service/pkg/internal/testutil"
	"github.com/storacha/indexing-service/pkg/service/claimlookup"
	"github.com/storacha/indexing-service/pkg/types"
	"github.com/stretchr/testify/require"
)

func TestWithCache__LookupClaim(t *testing.T) {

	// Create a test CID
	cachedCid := testutil.RandomCID().(cidlink.Link).Cid
	notCachedCid := testutil.RandomCID().(cidlink.Link).Cid
	// Create a cached claim
	cachedClaim := testutil.RandomLocationDelegation()
	notCachedClaim := testutil.RandomIndexDelegation()

	// sample error
	anError := errors.New("something went wrong")
	// Define test cases
	testCases := []struct {
		name          string
		claimCid      cid.Cid
		setErr        error
		getErr        error
		expectedErr   error
		baseLookup    *mockClaimLookup
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
			name:          "underlying lookup error",
			claimCid:      notCachedCid,
			expectedClaim: nil,
			baseLookup:    &mockClaimLookup{nil, anError},
			expectedErr:   fmt.Errorf("fetching underlying claim: %w", anError),
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
			lookup := tc.baseLookup
			if lookup == nil {
				lookup = &mockClaimLookup{notCachedClaim, nil}
			}
			// Create ClaimLookup instance
			cl := claimlookup.WithCache(lookup, mockStore)

			claim, err := cl.LookupClaim(context.Background(), tc.claimCid, *testutil.TestURL)
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

type mockClaimLookup struct {
	claim delegation.Delegation
	err   error
}

func (m *mockClaimLookup) LookupClaim(ctx context.Context, claimCid cid.Cid, fetchURL url.URL) (delegation.Delegation, error) {
	return m.claim, m.err
}
