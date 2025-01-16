package contentclaims_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/go-ucanto/core/ipld"
	"github.com/storacha/indexing-service/pkg/internal/testutil"
	"github.com/storacha/indexing-service/pkg/service/contentclaims"
	"github.com/storacha/indexing-service/pkg/types"
	"github.com/stretchr/testify/require"
)

func TestWithCache__Find(t *testing.T) {
	// Create a cached claim
	cachedClaim := testutil.RandomLocationDelegation()
	notCachedClaim := testutil.RandomIndexDelegation()

	// Create a test CID
	cachedCid := cachedClaim.Link()
	notCachedCid := notCachedClaim.Link()

	// sample error
	anError := errors.New("something went wrong")
	// Define test cases
	testCases := []struct {
		name          string
		claimCid      ipld.Link
		setErr        error
		getErr        error
		expectedErr   error
		baseFinder    *mockFinder
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
			name:          "Find error",
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
			expectedErr:   fmt.Errorf("caching claim: %w", anError),
			finalState: map[string]delegation.Delegation{
				cachedCid.String(): cachedClaim,
			},
		},
		{
			name:          "underlying find error",
			claimCid:      notCachedCid,
			expectedClaim: nil,
			baseFinder:    &mockFinder{nil, anError},
			expectedErr:   anError,
			finalState: map[string]delegation.Delegation{
				cachedCid.String(): cachedClaim,
			},
		},
	}

	// Run test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCache := &MockContentClaimsCache{
				setErr: tc.setErr,
				getErr: tc.getErr,
				claims: map[string]delegation.Delegation{
					cachedCid.String(): cachedClaim,
				},
			}
			// generate a test server for requests
			finder := tc.baseFinder
			if finder == nil {
				finder = &mockFinder{notCachedClaim, nil}
			}
			// Create ClaimLookup instance
			cl := contentclaims.WithCache(finder, mockCache)

			claim, err := cl.Find(context.Background(), tc.claimCid, testutil.TestURL)
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
			require.Equal(t, len(finalState), len(mockCache.claims))
			for c, claim := range mockCache.claims {
				expectedClaim := finalState[c]
				testutil.RequireEqualDelegation(t, expectedClaim, claim)
			}
		})
	}
}

// MockContentClaimsCache is a mock implementation of the ContentClaimsCache interface
type MockContentClaimsCache struct {
	setErr, getErr error
	claims         map[string]delegation.Delegation
}

var _ types.ContentClaimsCache = &MockContentClaimsCache{}

// SetExpirable implements types.ContentClaimsStore.
func (m *MockContentClaimsCache) SetExpirable(ctx context.Context, key cid.Cid, expires bool) error {
	return nil
}

func (m *MockContentClaimsCache) Get(ctx context.Context, claimCid cid.Cid) (delegation.Delegation, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	claim, exists := m.claims[claimCid.String()]
	if !exists {
		return nil, types.ErrKeyNotFound
	}
	return claim, nil
}

func (m *MockContentClaimsCache) Set(ctx context.Context, claimCid cid.Cid, claim delegation.Delegation, expires bool) error {
	if m.setErr != nil {
		return m.setErr
	}
	m.claims[claimCid.String()] = claim
	return nil
}

type mockFinder struct {
	claim delegation.Delegation
	err   error
}

func (m *mockFinder) Find(ctx context.Context, link ipld.Link, url *url.URL) (delegation.Delegation, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.claim == nil || m.claim.Link().String() != link.String() {
		return nil, types.ErrKeyNotFound
	}
	return m.claim, nil
}
