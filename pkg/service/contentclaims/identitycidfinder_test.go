package contentclaims_test

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/ipfs/go-cid"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	multihash "github.com/multiformats/go-multihash/core"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/go-ucanto/core/ipld"
	"github.com/storacha/indexing-service/pkg/internal/testutil"
	"github.com/storacha/indexing-service/pkg/service/contentclaims"
	"github.com/stretchr/testify/require"
)

func TestIdentityCidFinder__Find(t *testing.T) {
	// Create a cached claim
	identityCidClaim := testutil.RandomLocationDelegation()
	notIdentityCidClaim := testutil.RandomIndexDelegation()

	identityCiddata := testutil.Must(io.ReadAll(identityCidClaim.Archive()))(t)

	// Create a test CID
	identityCid := cidlink.Link{Cid: testutil.Must(cid.Prefix{
		Version:  1,
		Codec:    cid.Raw,
		MhType:   multihash.IDENTITY,
		MhLength: len(identityCiddata),
	}.Sum(identityCiddata))(t)}
	notIdentityCid := notIdentityCidClaim.Link()

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
			name:          "identity cid",
			claimCid:      identityCid,
			expectedClaim: identityCidClaim,
		},
		{
			name:          "not identity cid, successful fetch",
			claimCid:      notIdentityCid,
			expectedClaim: notIdentityCidClaim,
		},
		{
			name:          "underlying find error",
			claimCid:      notIdentityCid,
			expectedClaim: nil,
			baseFinder:    &mockFinder{nil, anError},
			expectedErr:   anError,
		},
	}

	// Run test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			// generate a test server for requests
			finder := tc.baseFinder
			if finder == nil {
				finder = &mockFinder{notIdentityCidClaim, nil}
			}
			// Create ClaimLookup instance
			cl := contentclaims.WithIdentityCids(finder)

			claim, err := cl.Find(context.Background(), tc.claimCid, *testutil.TestURL)
			if tc.expectedErr != nil {
				require.EqualError(t, err, tc.expectedErr.Error())
			} else {
				require.NoError(t, err)
			}
			testutil.RequireEqualDelegation(t, tc.expectedClaim, claim)
		})
	}
}
