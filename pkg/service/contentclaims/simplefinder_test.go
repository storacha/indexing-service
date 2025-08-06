package contentclaims_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/storacha/go-libstoracha/testutil"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/indexing-service/pkg/service/contentclaims"
	"github.com/stretchr/testify/require"
)

func TestSimpleFinder__Find(t *testing.T) {
	claim := testutil.RandomIndexDelegation(t)
	otherClaim := testutil.RandomIndexDelegation(t)

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
			name: "CID match failure",
			handler: func(w http.ResponseWriter, r *http.Request) {
				claimBytes := testutil.Must(io.ReadAll(otherClaim.Archive()))(t)
				testutil.Must(w.Write(claimBytes))(t)
			},
			expectedErr: fmt.Errorf("received delegation: %s, does not match expected delegation: %s", otherClaim.Link(), claim.Link()),
		},
		{
			name:        "failure",
			handler:     http.NotFound,
			expectedErr: errors.New("failure response fetching claim. URL: {url}, status: 404 Not Found, message: 404 page not found\n"),
		},
	}
	// Run test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			testServer := httptest.NewServer(tc.handler)
			defer func() { testServer.Close() }()
			// Create ClaimLookup instance
			cl := contentclaims.NewSimpleFinder(testServer.Client())
			claim, err := cl.Find(context.Background(), claim.Link(), testutil.Must(url.Parse(testServer.URL))(t))
			if tc.expectedErr != nil {
				require.EqualError(t, err, strings.ReplaceAll(tc.expectedErr.Error(), "{url}", testServer.URL))
			} else {
				require.NoError(t, err)
			}
			testutil.RequireEqualDelegation(t, tc.expectedClaim, claim)
		})
	}
}
