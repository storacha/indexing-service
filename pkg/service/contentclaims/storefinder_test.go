package contentclaims_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/go-ucanto/core/ipld"
	"github.com/storacha/indexing-service/pkg/internal/testutil"
	"github.com/storacha/indexing-service/pkg/service/contentclaims"
	"github.com/storacha/indexing-service/pkg/types"
	"github.com/stretchr/testify/require"
)

func TestWithStore__Find(t *testing.T) {
	// Create a cached claim
	storedClaim := testutil.RandomLocationDelegation()
	notStoredClaim := testutil.RandomIndexDelegation()

	// Create a test CID
	storedCid := storedClaim.Link()
	notStoredCid := notStoredClaim.Link()

	// sample error
	anError := errors.New("something went wrong")
	// Define test cases
	testCases := []struct {
		name          string
		claimCid      ipld.Link
		getErr        error
		expectedErr   error
		baseFinder    *mockFinder
		expectedClaim delegation.Delegation
	}{
		{
			name:          "Claim stored",
			claimCid:      storedCid,
			expectedClaim: storedClaim,
		},
		{
			name:          "Claim not stored, successful fetch",
			claimCid:      notStoredCid,
			expectedClaim: notStoredClaim,
		},
		{
			name:          "Find error",
			claimCid:      storedCid,
			expectedClaim: nil,
			getErr:        anError,
			expectedErr:   errors.Join(fmt.Errorf("reading from claim store: %w", anError), types.ErrKeyNotFound),
		},
		{
			name:          "underlying find error",
			claimCid:      notStoredCid,
			expectedClaim: nil,
			baseFinder:    &mockFinder{nil, anError},
			expectedErr:   anError,
		},
	}

	// Run test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockStore := &MockContentClaimsStore{
				getErr: tc.getErr,
				claims: map[string]delegation.Delegation{
					storedCid.String(): storedClaim,
				},
			}
			// generate a test server for requests
			finder := tc.baseFinder
			if finder == nil {
				finder = &mockFinder{notStoredClaim, nil}
			}
			// Create ClaimLookup instance
			cl := contentclaims.WithStore(finder, mockStore)

			claim, err := cl.Find(context.Background(), tc.claimCid, testutil.TestURL)
			if tc.expectedErr != nil {
				require.EqualError(t, err, tc.expectedErr.Error())
			} else {
				require.NoError(t, err)
			}
			testutil.RequireEqualDelegation(t, tc.expectedClaim, claim)
		})
	}
}

// MockContentClaimsStore is a mock implementation of the ContentClaimsStore interface
type MockContentClaimsStore struct {
	setErr, getErr error
	claims         map[string]delegation.Delegation
}

var _ types.ContentClaimsStore = &MockContentClaimsStore{}

func (m *MockContentClaimsStore) Get(ctx context.Context, key ipld.Link) (delegation.Delegation, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	claim, exists := m.claims[key.String()]
	if !exists {
		return nil, types.ErrKeyNotFound
	}
	return claim, nil
}

func (m *MockContentClaimsStore) Put(ctx context.Context, key ipld.Link, claim delegation.Delegation) error {
	if m.setErr != nil {
		return m.setErr
	}
	m.claims[key.String()] = claim
	return nil
}
