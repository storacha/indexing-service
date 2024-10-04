package claimlookup_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/indexing-service/pkg/internal/testutil"
	"github.com/storacha/indexing-service/pkg/service/claimlookup"
	"github.com/stretchr/testify/require"
)

func TestClaimLookup__LookupClaim(t *testing.T) {
	cid := testutil.RandomCID().(cidlink.Link).Cid
	claim := testutil.RandomIndexDelegation()
	// sample error
	testCases := []struct {
		name          string
		handler       http.HandlerFunc
		expectedErr   error
		expectedClaim delegation.Delegation
	}{
		{
			name: "success fetch",
			handler: func(w http.ResponseWriter, r *http.Request) {
				claimBytes := testutil.Must(io.ReadAll(claim.Archive()))(t)
				testutil.Must(w.Write(claimBytes))(t)
			},
			expectedClaim: claim,
		},
		{
			name:        "failure",
			handler:     http.NotFound,
			expectedErr: errors.New("failure response fetching claim. status: 404 Not Found, message: 404 page not found\n"),
		},
	}
	// Run test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			testServer := httptest.NewServer(tc.handler)
			defer func() { testServer.Close() }()
			// Create ClaimLookup instance
			cl := claimlookup.NewClaimLookup(testServer.Client())
			claim, err := cl.LookupClaim(context.Background(), cid, *testutil.Must(url.Parse(testServer.URL))(t))
			if tc.expectedErr != nil {
				require.EqualError(t, err, tc.expectedErr.Error())
			} else {
				require.NoError(t, err)
			}
			testutil.RequireEqualDelegation(t, tc.expectedClaim, claim)
		})
	}
}
